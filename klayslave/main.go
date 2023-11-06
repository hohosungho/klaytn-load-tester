package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	blockchain "github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/erc20TransferTC"
	"github.com/klaytn/klaytn-load-tester/klayslave/erc20TransferWithReceiptTimeTC"
	"github.com/klaytn/klaytn-load-tester/klayslave/task"
	"github.com/klaytn/klaytn-load-tester/klayslave/transferSignedTc"
	"github.com/myzhan/boomer"
	"log"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:generate abigen --sol cpuHeavyTC/CPUHeavy.sol --pkg cpuHeavyTC --out cpuHeavyTC/CPUHeavy.go
//go:generate abigen --sol userStorageTC/UserStorage.sol --pkg userStorageTC --out userStorageTC/UserStorage.go

// Dedicated and fixed private key used to deploy a smart contract for ERC20 and ERC721 value transfer performance test.
var ERC20DeployPrivateKeyStr = "eb2c84d41c639178ff26a81f488c196584d678bb1390cc20a3aeb536f3969a98"

// Sets build options from ldflags.
var (
	Version   = "1.0.0"
	Commit    string
	Branch    string
	Tag       string
	BuildDate string
	BuildUser string
)

var (
	coinbasePrivatekey = ""
	keypathsStr        string
	keypaths           []string
	gCli               *ethclient.Client
	gEndpoint          string

	coinbase    *account.Account
	newCoinbase *account.Account

	nUserForUnsigned    = 5 //number of virtual user account for unsigned tx
	accGrpForUnsignedTx []*account.Account

	nUserForSigned    = 5
	accGrpForSignedTx []*account.Account

	nUserForNewAccounts  = 5
	accGrpForNewAccounts []*account.Account

	activeFromUserPercent = 100
	activeToUserPercent   = 100

	SmartContractAccount *account.Account

	tcStr     string
	tcStrList []string

	chargeValue *big.Int

	gasPrice *big.Int

	aggregateTcName bool

	skipCharging bool
)

func Create(endpoint string) *ethclient.Client {
	c, err := ethclient.Dial(endpoint)
	if err != nil {
		log.Fatalf("Failed to connect RPC: %v", err)
	}
	return c
}

func inTheTCList(tcNames ...string) bool {
	for _, tcName := range tcNames {
		for _, tc := range tcStrList {
			if tcName == tc {
				return true
			}
		}
	}

	return false
}

func prepareTestAccountsAndContracts(accGrp map[common.Address]*account.Account) {
	chargeEtherToTestAccounts(accGrp)
	prepareERC20Transfer(accGrp)
}

func chargeEtherToTestAccounts(accGrp map[common.Address]*account.Account) {
	if skipCharging {
		log.Println("Skip charging KLAY to test accounts")
		return
	}

	log.Printf("Start charging KLAY to test accounts")
	numChargedAcc := 0
	lastFailedNum := 0
	for _, acc := range accGrp {
		for {
			_, _, err := newCoinbase.TransferSignedTxReturnTx(true, gCli, acc, chargeValue)
			if err == nil {
				break // Success, move to next account.
			}
			log.Printf("transfer failed due to err : %v, charged:%d, failed :%d, storedNonce: %d, fetchNonce: %d", err, numChargedAcc, lastFailedNum, newCoinbase.GetNonce(gCli), newCoinbase.GetNonceFromBlock(gCli))
			numChargedAcc, lastFailedNum = estimateRemainingTimeWithErr(accGrp, numChargedAcc, lastFailedNum, err)
		}
		numChargedAcc++
		if numChargedAcc%1000 == 0 {
			log.Printf("ChargeSummary charged:%d", numChargedAcc)
		}
	}

	log.Printf("Finished charghing KLAY to %d test account(s), Total %d transactions are sent.\n", len(accGrp), numChargedAcc)
}

func prepareERC20Transfer(accGrp map[common.Address]*account.Account) {
	if !inTheTCList(erc20TransferTC.Name, erc20TransferWithReceiptTimeTC.Name) {
		return
	}
	erc20DeployAcc := account.GetAccountFromKey(0, ERC20DeployPrivateKeyStr)
	log.Printf("prepareERC20Transfer", "addr", erc20DeployAcc.GetAddress().String())
	chargeEtherToTestAccounts(map[common.Address]*account.Account{erc20DeployAcc.GetAddress(): erc20DeployAcc})

	// A smart contract for ERC20 value transfer performance TC.
	erc20TransferTC.SmartContractAccount = deploySingleSmartContract(erc20DeployAcc, erc20DeployAcc.DeployERC20, "ERC20 Performance Test Contract")
	newCoinBaseAccountMap := map[common.Address]*account.Account{newCoinbase.GetAddress(): newCoinbase}
	firstChargeTokenToTestAccounts(newCoinBaseAccountMap, erc20TransferTC.SmartContractAccount.GetAddress(), erc20DeployAcc.TransferERC20, big.NewInt(1e10))

	chargeTokenToTestAccounts(accGrp, erc20TransferTC.SmartContractAccount.GetAddress(), newCoinbase.TransferERC20, big.NewInt(1e3))
}

type tokenChargeFunc func(initialCharge bool, c *ethclient.Client, tokenContractAddr common.Address, recipient *account.Account, value *big.Int) (*types.Transaction, *big.Int, error)

// firstChargeTokenToTestAccounts charges initially generated tokens to newCoinbase account for further testing.
// As this work is done simultaneously by different slaves, this should be done in "try and check" manner.
func firstChargeTokenToTestAccounts(accGrp map[common.Address]*account.Account, tokenContractAddr common.Address, tokenChargeFn tokenChargeFunc, tokenChargeAmount *big.Int) {
	log.Printf("Start initial token charging to new coinbase")

	numChargedAcc := 0
	for _, recipientAccount := range accGrp {
		for {
			tx, _, err := tokenChargeFn(true, gCli, tokenContractAddr, recipientAccount, tokenChargeAmount)
			for err != nil {
				log.Printf("Failed to execute %s: err %s", tx.Hash().String(), err.Error())
				time.Sleep(1 * time.Second) // Mostly, the err is `txpool is full`, retry after a while.
				tx, _, err = tokenChargeFn(true, gCli, tokenContractAddr, recipientAccount, tokenChargeAmount)
			}
			ctx, cancelFn := context.WithTimeout(context.Background(), 20*time.Second)
			receipt, err := bind.WaitMined(ctx, gCli, tx)
			if err != nil {
				log.Printf("Failed to wait. txhash: %s err %s", tx.Hash().String(), err.Error())
			}
			cancelFn()
			if receipt != nil {
				if receipt.Status == 0 {
					log.Println("Failed to charge accounts")
				}
				break
			}
		}
		numChargedAcc++
	}
	log.Printf("Finished initial token charging to %d new coinbase account(s), Total %d transactions are sent.\n", len(accGrp), numChargedAcc)
}

// chargeTokenToTestAccounts charges default token to the test accounts for testing.
// As it is done independently among the slaves, it has simpler logic than firstChargeTokenToTestAccounts.
func chargeTokenToTestAccounts(accGrp map[common.Address]*account.Account, tokenContractAddr common.Address, tokenChargeFn tokenChargeFunc, tokenChargeAmount *big.Int) {
	log.Printf("Start charging tokens to test accounts")

	numChargedAcc := 0
	lastFailedNum := 0
	wg := sync.WaitGroup{}
	for _, recipientAccount := range accGrp {
		wg.Add(1)
		go func(acc *account.Account) {
			for {
				tx, _, err := tokenChargeFn(false, gCli, tokenContractAddr, acc, tokenChargeAmount)
				ctx, cancelFn := context.WithTimeout(context.Background(), 20*time.Second)
				receipt, err := bind.WaitMined(ctx, gCli, tx)
				if err != nil {
					log.Printf("Failed to wait. txhash: %s err %s", tx.Hash().String(), err.Error())
				}
				cancelFn()
				if receipt != nil {
					if receipt.Status == 0 {
						log.Println("Failed to charge accounts")
					}
				}
				if err == nil {
					wg.Done()
					break // Success, move to next account.
				}
				numChargedAcc, lastFailedNum = estimateRemainingTime(accGrp, numChargedAcc, lastFailedNum)
			}
		}(recipientAccount)
		numChargedAcc++
	}
	wg.Wait()
	log.Printf("Finished charging tokens to %d test account(s), Total %d transactions are sent.\n", len(accGrp), numChargedAcc)
}

func estimateRemainingTime(accGrp map[common.Address]*account.Account, numChargedAcc, lastFailedNum int) (int, int) {
	if lastFailedNum > 0 {
		// Not 1st failed cases.
		TPS := (numChargedAcc - lastFailedNum) / 5 // TPS of only this slave during `txpool is full` situation.
		lastFailedNum = numChargedAcc

		if TPS <= 5 {
			log.Printf("Retry to charge test account #%d. But it is too slow. %d TPS\n", numChargedAcc, TPS)
		} else {
			remainTime := (len(accGrp) - numChargedAcc) / TPS
			remainHour := remainTime / 3600
			remainMinute := (remainTime % 3600) / 60

			log.Printf("Retry to charge test account #%d. Estimated remaining time: %d hours %d mins later\n", numChargedAcc, remainHour, remainMinute)
		}
	} else {
		// 1st failed case.
		lastFailedNum = numChargedAcc
		log.Printf("Retry to charge test account #%d.\n", numChargedAcc)
	}
	log.Printf("Sleeping for 5 sec due to low TPS")
	time.Sleep(5 * time.Second) // Mostly, the err is `txpool is full`, retry after a while.
	log.Printf("Awaking from sleeping for 5 sec due to low TPS")
	return numChargedAcc, lastFailedNum
}

func estimateRemainingTimeWithErr(accGrp map[common.Address]*account.Account, numChargedAcc, lastFailedNum int, err error) (int, int) {
	sleep := time.Duration(0)
	if strings.Contains(err.Error(), "txpool is full") {
		time.Sleep(5 * time.Second)
	}
	if lastFailedNum > 0 {
		// Not 1st failed cases.
		TPS := (numChargedAcc - lastFailedNum) / 5 // TPS of only this slave during `txpool is full` situation.
		lastFailedNum = numChargedAcc

		if TPS <= 5 {
			log.Printf("Retry to charge test account #%d. But it is too slow. %d TPS\n", numChargedAcc, TPS)
			sleep = time.Second
		} else {
			remainTime := (len(accGrp) - numChargedAcc) / TPS
			remainHour := remainTime / 3600
			remainMinute := (remainTime % 3600) / 60

			log.Printf("Retry to charge test account #%d. Estimated remaining time: %d hours %d mins later\n", numChargedAcc, remainHour, remainMinute)
		}
	} else {
		// 1st failed case.
		lastFailedNum = numChargedAcc
		log.Printf("Retry to charge test account #%d.\n", numChargedAcc)
	}
	if sleep != 0 {
		log.Printf("Sleeping for %s due to low TPS", sleep)
		time.Sleep(sleep) // Mostly, the err is `txpool is full`, retry after a while.
	}
	return numChargedAcc, lastFailedNum
}

type contractDeployFunc func(c *ethclient.Client, to *account.Account, value *big.Int, humanReadable bool) (common.Address, *types.Transaction, *big.Int, error)

// deploySmartContract deploys smart contracts by the number of locust slaves.
// In other words, each slave owns its own contract for testing.
func deploySmartContract(contractDeployFn contractDeployFunc, contractName string) *account.Account {
	addr, lastTx, _, err := contractDeployFn(gCli, SmartContractAccount, common.Big0, false)
	for err != nil {
		log.Printf("Failed to deploy a %s: err %s", contractName, err.Error())
		time.Sleep(5 * time.Second) // Mostly, the err is `txpool is full`, retry after a while.
		addr, lastTx, _, err = contractDeployFn(gCli, SmartContractAccount, common.Big0, false)
	}

	log.Printf("Start waiting the receipt of the %s tx(%v).\n", contractName, lastTx.Hash().String())
	bind.WaitMined(context.Background(), gCli, lastTx)

	deployedContract := account.NewEthereumAccountWithAddr(addr)
	log.Printf("%s has been deployed to : %s\n", contractName, addr.String())
	return deployedContract
}

// deploySingleSmartContract deploys only one smart contract among the slaves.
// It the contract is already deployed by other slave, it just calculates the address of the contract.
func deploySingleSmartContract(erc20DeployAcc *account.Account, contractDeployFn contractDeployFunc, contractName string) *account.Account {
	addr, lastTx, _, err := contractDeployFn(gCli, SmartContractAccount, common.Big0, false)
	log.Println("contract address: " + addr.Hex())
	for err != nil {
		if err == account.AlreadyDeployedErr {
			erc20Addr := crypto.CreateAddress(erc20DeployAcc.GetAddress(), 0)
			return account.NewEthereumAccountWithAddr(erc20Addr)
		}
		if strings.HasPrefix(err.Error(), "Known transaction") || strings.EqualFold(err.Error(), blockchain.ErrNonceTooLow.Error()) || strings.EqualFold(err.Error(), txpool.ErrReplaceUnderpriced.Error()) {
			log.Printf("Failed to deploy a %s: err %s", contractName, err.Error())
			time.Sleep(5 * time.Second) // Mostly, the err is `txpool is full`, retry after a while.
			addr, lastTx, _, err = contractDeployFn(gCli, SmartContractAccount, common.Big0, false)
		}
	}

	log.Printf("Start waiting the receipt of the %s tx(%v).\n", contractName, lastTx.Hash().String())
	bind.WaitDeployed(context.Background(), gCli, lastTx)

	deployedContract := account.NewEthereumAccountWithAddr(addr)
	log.Printf("%s has been deployed to : %s\n", contractName, addr.String())

	return deployedContract
}

func prepareAccounts() error {
	totalChargeValue := new(big.Int)
	multiply := big.NewInt(1)
	if !skipCharging {
		multiply = big.NewInt(int64(nUserForUnsigned + nUserForSigned + nUserForNewAccounts + 1))
	}
	totalChargeValue.Mul(chargeValue, multiply)

	// Import coinbase Account
	coinbase = account.GetAccountFromKey(0, coinbasePrivatekey)
	newCoinbase = account.NewAccount(0)

	if !skipCharging {
		for {
			coinbase.GetNonceFromBlock(gCli)
			hash, _, err := coinbase.TransferSignedTx(gCli, newCoinbase, totalChargeValue)
			if err != nil {
				log.Printf("%v: charge newCoinbase fail: %v\n", os.Getpid(), err)
				time.Sleep(1000 * time.Millisecond)
				continue
			}

			log.Printf("%v : charge newCoinbase: %v, Txhash=%v\n", os.Getpid(), newCoinbase.GetAddress().String(), hash.String())

			getReceipt := false
			// After this loop waiting for 10 sec, It will retry to charge with new nonce.
			// it means another node stole the nonce.
			for i := 0; i < 5; i++ {
				time.Sleep(2000 * time.Millisecond)
				ctx := context.Background()

				//_, err := gCli.TransactionReceipt(ctx, hash)
				//if err != nil {
				//	getReceipt = true
				//	log.Printf("%v : charge newCoinbase success: %v\n", os.Getpid(), newCoinbase.GetAddress().String())
				//	break
				//}
				//log.Printf("%v : charge newCoinbase waiting: %v\n", os.Getpid(), newCoinbase.GetAddress().String())

				val, err := gCli.BalanceAt(ctx, newCoinbase.GetAddress(), nil)
				if err == nil {
					if val.Cmp(big.NewInt(0)) == 1 {
						getReceipt = true
						log.Printf("%v : charge newCoinbase success: %v, balance=%v peb\n", os.Getpid(), newCoinbase.GetAddress().String(), val.String())
						break
					}
					log.Printf("%v : charge newCoinbase waiting: %v\n", os.Getpid(), newCoinbase.GetAddress().String())
				} else {
					log.Printf("%v : check banalce err: %v\n", os.Getpid(), err)
				}
			}

			if getReceipt {
				break
			}
		}
	}

	var (
		accountReader account.Reader
		err           error
	)
	if keypathsStr == "" {
		accountReader = account.NewReader()
	} else {
		keypaths = strings.Split(keypathsStr, ",")
		accountReader, err = account.NewFileReader(keypaths, false)
		if err != nil {
			return err
		}
	}

	// Create test account pool
	println("Unsigned Account Group Preparation...")
	accGrpForUnsignedTx, err = accountReader.Read(nUserForUnsigned)
	if err != nil {
		return err
	}

	println("Signed Account Group Preparation...")
	accGrpForSignedTx, err = accountReader.Read(nUserForSigned)
	if err != nil {
		return err
	}

	println("New account group preparation...")
	accGrpForNewAccounts, err = accountReader.Read(nUserForNewAccounts)
	if err != nil {
		return err
	}
	return nil
}

func initArgs(tcNames string) (terminate bool) {
	versionPtr := flag.Bool("version", false, "print build options")
	gEndpointPtr := flag.String("endpoint", "http://localhost:8545", "Target EndPoint")
	nUserForSignedPtr := flag.Int("vusigned", nUserForSigned, "num of test account for signed Tx TC")
	nUserForUnsignedPtr := flag.Int("vuunsigned", nUserForUnsigned, "num of test account for unsigned Tx TC")
	activeUserPercentPtr := flag.Int("activepercent", 0, "percent of active accounts (default: 100)")
	activeFromUserPercentPtr := flag.Int("activefrompercent", 0, "percent of active from accounts")
	activeToUserPercentPtr := flag.Int("activetopercent", 0, "percent of active to accounts")

	keyPtr := flag.String("key", "", "privatekey of coinbase")
	chargeETHAmountPtr := flag.String("charge", "1ether", `charging amount for each test account. e.g) 100 => 100klay / 100peb / 10klay. default: 1klay`)
	flag.StringVar(&keypathsStr, "keypaths", "", "pre-defined private key file paths for test account")
	flag.StringVar(&tcStr, "tc", tcNames, "tasks which user want to run, multiple tasks are separated by comma.")
	flag.BoolVar(&aggregateTcName, "aggregatetcname", false, "aggregate testcase names i.e. does not append the endpoint")
	flag.BoolVar(&skipCharging, "skipcharging", false, "skip charging with test accounts")

	flag.Parse()

	if *versionPtr || (len(os.Args) >= 2 && os.Args[1] == "version") {
		printVersion()
		return true
	}

	if *keyPtr == "" {
		log.Fatal("key argument is not defined. You should set the key for coinbase.\n example) klaytc -key='2ef07640fd8d3f568c23185799ee92e0154bf08ccfe5c509466d1d40baca3430'")
	}

	// for TC Selection
	if tcStr != "" {
		// Run tasks without connecting to the master.
		tcStrList = strings.Split(tcStr, ",")
	}

	var err error

	gEndpoint = *gEndpointPtr
	nUserForSigned = *nUserForSignedPtr
	nUserForUnsigned = *nUserForUnsignedPtr
	coinbasePrivatekey = *keyPtr
	activeFromUserPercent, activeToUserPercent, err = parseActiveUser(*activeUserPercentPtr, *activeFromUserPercentPtr, *activeToUserPercentPtr)
	if err != nil {
		log.Fatal(err)
	}

	parsedCharge, err := parseAmount(*chargeETHAmountPtr)
	if err != nil {
		log.Fatal(err)
	}
	chargeValue = new(big.Int)
	chargeValue.Set(parsedCharge)

	fmt.Println("Arguments are set like the following:")
	fmt.Printf("- Target EndPoint = %v\n", gEndpoint)
	fmt.Printf("- nUserForSigned = %v\n", nUserForSigned)
	fmt.Printf("- nUserForUnsigned = %v\n", nUserForUnsigned)
	fmt.Printf("- activeFromUserPercent = %v\n", activeFromUserPercent)
	fmt.Printf("- activeToUserPercent = %v\n", activeToUserPercent)
	fmt.Printf("- coinbasePrivatekey = %v\n", coinbasePrivatekey)
	fmt.Printf("- charging KLAY Amount = %s\n", *chargeETHAmountPtr)
	fmt.Printf("- tc = %v\n", tcStr)
	return false
}

func parseActiveUser(activeUserPercent, activeFromUserPercent, activeToUserPercent int) (int, int, error) {
	if activeUserPercent == 0 && activeFromUserPercent == 0 && activeToUserPercent == 0 {
		return 100, 100, nil
	}
	// If provide --activepercent, then use --activepercent.
	if activeUserPercent != 0 {
		if activeFromUserPercent != 0 || activeToUserPercent != 0 {
			return 0, 0, errors.New("require either (--activepercent) or (--activefrompercent and --activetopercent) flag")
		}
		return activeUserPercent, activeUserPercent, nil
	}

	if activeFromUserPercent == 0 || activeToUserPercent == 0 {
		return 0, 0, errors.New("both --activefrompercent and --activetopercent are required")
	}
	return activeFromUserPercent, activeToUserPercent, nil
}

func parseAmount(val string) (*big.Int, error) {
	idx := -1
	for i := range val {
		if val[i] < '0' || val[i] > '9' {
			break
		}
		idx = i
	}
	if idx < 0 {
		return nil, fmt.Errorf("invalid amount: %s", val)
	}

	valueStr, unit := val[0:idx+1], val[idx+1:]
	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return nil, err
	}
	if unit == "" {
		unit = "ether"
	}
	switch unit {
	case "wei":
		return new(big.Int).Mul(big.NewInt(value), big.NewInt(params.Wei)), nil
	case "gwei":
		return new(big.Int).Mul(big.NewInt(value), big.NewInt(params.GWei)), nil
	case "ether":
		return new(big.Int).Mul(big.NewInt(value), big.NewInt(params.Ether)), nil
	default:
		return nil, fmt.Errorf("unknown unit: %s", unit)
	}
}

func updateChainID() {
	fmt.Println("Updating ChainID from RPC")
	for {
		ctx := context.Background()
		chainID, err := gCli.ChainID(ctx)

		if err == nil {
			fmt.Println("chainID :", chainID)
			account.SetChainID(chainID)
			break
		}
		fmt.Println("Retrying updating chainID... ERR: ", err)

		time.Sleep(2 * time.Second)
	}
}

func updateGasPrice() {
	gasPrice = big.NewInt(0)
	//fmt.Println("Updating GasPrice from RPC")
	//for {
	//	ctx := context.Background()
	//	gp, err := gCli.SuggestGasPrice(ctx)
	//
	//	if err == nil {
	//		gasPrice = gp
	//		fmt.Println("gas price :", gasPrice.String())
	//		break
	//	}
	//	fmt.Println("Retrying updating GasPrice... ERR: ", err)
	//
	//	time.Sleep(2 * time.Second)
	//}
	account.SetGasPrice(gasPrice)
}

func setRLimit(resourceType int, val uint64) error {
	if runtime.GOOS == "darwin" {
		return nil
	}
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(resourceType, &rLimit)
	if err != nil {
		return err
	}
	rLimit.Cur = val
	err = syscall.Setrlimit(resourceType, &rLimit)
	if err != nil {
		return err
	}
	return nil
}

// initTCList initializes TCs and returns a slice of TCs.
func initTCList() (taskSet []*task.ExtendedTask) {
	taskSet = append(taskSet, &task.ExtendedTask{
		Name:    "transferSignedTx",
		Weight:  10,
		Fn:      transferSignedTc.Run,
		Init:    transferSignedTc.Init,
		AccGrp:  accGrpForSignedTx, //[:nUserForSigned/2-1],
		EndPint: gEndpoint,
	})

	//taskSet = append(taskSet, &task.ExtendedTask{
	//	Name:    "transferSignedWithReceiptTime",
	//	Weight:  10,
	//	Fn:      transferSignedWithReceiptTimeTc.Run,
	//	Init:    transferSignedWithReceiptTimeTc.Init,
	//	AccGrp:  accGrpForSignedTx, //[:nUserForSigned/2-1],
	//	EndPint: gEndpoint,
	//})
	//
	//taskSet = append(taskSet, &task.ExtendedTask{
	//	Name:    "transferSignedWithReceiptTimeAsync",
	//	Weight:  10,
	//	Fn:      transferSignedWithReceiptTimeAsyncTc.Run,
	//	Init:    transferSignedWithReceiptTimeAsyncTc.Init,
	//	Stop:    transferSignedWithReceiptTimeAsyncTc.Stop,
	//	AccGrp:  accGrpForSignedTx, //[:nUserForSigned/2-1],
	//	EndPint: gEndpoint,
	//})

	taskSet = append(taskSet, &task.ExtendedTask{
		Name:    erc20TransferTC.Name,
		Weight:  10,
		Fn:      erc20TransferTC.Run,
		Init:    erc20TransferTC.Init,
		AccGrp:  accGrpForSignedTx,
		Stop:    erc20TransferTC.Stop,
		EndPint: gEndpoint,
	})

	taskSet = append(taskSet, &task.ExtendedTask{
		Name:    erc20TransferWithReceiptTimeTC.Name,
		Weight:  10,
		Fn:      erc20TransferWithReceiptTimeTC.Run,
		Init:    erc20TransferWithReceiptTimeTC.Init,
		AccGrp:  accGrpForSignedTx,
		Stop:    erc20TransferWithReceiptTimeTC.Stop,
		EndPint: gEndpoint,
	})

	return taskSet
}

func printVersion() {
	version := Version
	if len(Commit) >= 7 {
		version += "-" + Commit[:7]
	}
	if Tag != "" && Tag != "undefined" {
		version = Tag
	}
	fmt.Printf("Version :\t%s\n", version)
	fmt.Printf("git.Branch :\t%s\n", Branch)
	fmt.Printf("git.Commit :\t%s\n", Commit)
	fmt.Printf("git.Tag :\t%s\n", Tag)
	fmt.Printf("build.Date :\t%s\n", BuildDate)
	fmt.Printf("build.User :\t%s\n", BuildUser)
}

func main() {
	// Configure http.DefaultTransport's idle connections per host to prevent closing connections.
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 100
	// Call initTCList to get all TC names
	taskSet := initTCList()

	var tcNames string
	for i, task := range taskSet {
		if i != 0 {
			tcNames += ","
		}
		tcNames += task.Name
	}

	if terminate := initArgs(tcNames); terminate {
		os.Exit(0)
	}

	// Create Cli pool
	gCli = Create(gEndpoint)

	// Update chainID
	updateChainID()

	// Update gasPrice
	updateGasPrice()

	// Set coinbase & Create Test Account
	if err := prepareAccounts(); err != nil {
		log.Printf("failed to prepare account. err: %v", err)
		os.Exit(1)
	}

	// Call initTCList again to actually define all TCs
	taskSet = initTCList()

	var filteredTask []*task.ExtendedTask

	println("Adding tasks")
	for _, task := range taskSet {
		if task.Name == "" {
			continue
		} else {
			flag := false
			for _, tc := range tcStrList {
				values := strings.Split(tc, ":")
				name := values[0]
				weight := 10
				if len(values) == 2 {
					w, err := strconv.ParseInt(values[1], 10, 32)
					if err != nil {
						log.Fatalf("invalid testcase name with weight: %s. err: %v", tc, err)
					}
					weight = int(w)
				}
				if name == task.Name {
					flag = true
					task.Weight = weight
					break
				}
			}
			if flag {
				filteredTask = append(filteredTask, task)
				println(fmt.Sprintf("=> %s(%d) task is added.", task.Name, task.Weight))
			}
		}
	}

	// Charge AccGrp
	accGrp := make(map[common.Address]*account.Account)
	for _, task := range filteredTask {
		for _, acc := range task.AccGrp {
			_, exist := accGrp[acc.GetAddress()]
			if !exist {
				accGrp[acc.GetAddress()] = acc
			}
		}

	}
	prepareTestAccountsAndContracts(accGrp)

	if len(filteredTask) == 0 {
		log.Fatal("No Tc is set. Please set TcList. \nExample argument) -tc='" + tcNames + "'")
	}

	println("Initializing tasks")
	var filteredBoomerTask []*boomer.Task
	for _, t := range filteredTask {
		t.Init(task.Params{
			AccGrp:          t.AccGrp,
			Endpoint:        t.EndPint,
			GasPrice:        gasPrice,
			AggregateTcName: aggregateTcName,
			ActiveFromUsers: len(t.AccGrp) * activeFromUserPercent / 100,
			ActiveToUsers:   len(t.AccGrp) * activeToUserPercent / 100,
		})
		filteredBoomerTask = append(filteredBoomerTask, &boomer.Task{t.Weight, t.Fn, t.Name})
		println("=> " + t.Name + " task is initialized.")
	}

	setRLimit(syscall.RLIMIT_NOFILE, 1024*400)

	boomer.Events.Subscribe("boomer:stop", func() {
		for _, t := range filteredTask {
			if t.Stop != nil {
				t.Stop()
			}
		}
	})

	// Locust Slave Run
	boomer.Run(filteredBoomerTask...)
	//boomer.Run(cpuHeavyTx)

	log.Printf("Terminate boomer")
}
