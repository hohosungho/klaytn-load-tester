package main

//go:generate abigen --sol cpuHeavyTC/CPUHeavy.sol --pkg cpuHeavyTC --out cpuHeavyTC/CPUHeavy.go
//go:generate abigen --sol userStorageTC/UserStorage.sol --pkg userStorageTC --out userStorageTC/UserStorage.go

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/transferSignedTc"

	"github.com/ethereum/go-ethereum/common"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/params"
	"github.com/myzhan/boomer"
)

// sets build options from ldflags.
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
	gCli               *client.Client
	gEndpoint          string

	coinbase    *account.Account
	newCoinbase *account.Account

	nUserForSigned    = 5
	accGrpForSignedTx []*account.Account

	activeUserPercent = 100

	tcStr     string
	tcStrList []string

	chargeValue *big.Int

	gasPrice *big.Int
	baseFee  *big.Int
)

type ExtendedTask struct {
	Name    string
	Weight  int
	Fn      func()
	Init    func(accs []*account.Account, endpoint string, gp *big.Int)
	AccGrp  []*account.Account
	EndPint string
}

func Create(endpoint string) *client.Client {
	c, err := client.Dial(endpoint)
	if err != nil {
		log.Fatalf("Failed to connect RPC: %v", err)
	}
	return c
}

func prepareTestAccountsAndContracts(accGrp map[common.Address]*account.Account) {
	// First, charging KLAY to the test accounts.
	chargeKLAYToTestAccounts(accGrp)
}

func chargeKLAYToTestAccounts(accGrp map[common.Address]*account.Account) {
	log.Printf("Start charging KLAY to test accounts")

	numChargedAcc := 0
	lastFailedNum := 0
	for _, acc := range accGrp {
		for {
			_, _, err := newCoinbase.TransferSignedTxReturnTx(true, gCli, acc, chargeValue)
			if err == nil {
				break // Success, move to next account.
			}
			numChargedAcc, lastFailedNum = estimateRemainingTime(accGrp, numChargedAcc, lastFailedNum)
		}
		numChargedAcc++
	}

	log.Printf("Finished charging KLAY to %d test account(s), Total %d transactions are sent.\n", len(accGrp), numChargedAcc)
}

type tokenChargeFunc func(initialCharge bool, c *client.Client, tokenContractAddr common.Address, recipient *account.Account, value *big.Int) (*types.Transaction, *big.Int, error)

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
	time.Sleep(5 * time.Second) // Mostly, the err is `txpool is full`, retry after a while.
	return numChargedAcc, lastFailedNum
}

func prepareAccounts() {
	totalChargeValue := new(big.Int)
	totalChargeValue.Mul(chargeValue, big.NewInt(int64(nUserForSigned+1)))

	// Import coinbase Account
	coinbase = account.GetAccountFromKey(0, coinbasePrivatekey)
	newCoinbase = account.NewAccount(0)

	if len(chargeValue.Bits()) != 0 {
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
					log.Printf("%v : check balance err: %v\n", os.Getpid(), err)
				}
			}

			if getReceipt {
				break
			}
		}
	}

	println("Signed Account Group Preparation...")
	//bar = pb.StartNew(nUserForSigned)

	for i := 0; i < nUserForSigned; i++ {
		accGrpForSignedTx = append(accGrpForSignedTx, account.NewAccount(i))
		fmt.Printf("%v\n", accGrpForSignedTx[i].GetAddress().String())
		//bar.Increment()
	}
}

func initArgs(tcNames string) {
	chargeKLAYAmount := 1000000000
	gEndpointPtr := flag.String("endpoint", "http://localhost:8545", "Target EndPoint")
	nUserForSignedPtr := flag.Int("vusigned", nUserForSigned, "num of test account for signed Tx TC")
	activeUserPercentPtr := flag.Int("activepercent", activeUserPercent, "percent of active accounts")
	keyPtr := flag.String("key", "", "privatekey of coinbase")
	versionPtr := flag.Bool("version", false, "show version number")
	httpMaxIdleConnsPtr := flag.Int("http.maxidleconns", 100, "maximum number of idle connections in default http client")
	flag.StringVar(&tcStr, "tc", tcNames, "tasks which user want to run, multiple tasks are separated by comma.")

	flag.Parse()

	if *versionPtr || (len(os.Args) >= 2 && os.Args[1] == "version") {
		printVersion()
		os.Exit(0)
	}

	if *keyPtr == "" {
		log.Fatal("key argument is not defined. You should set the key for coinbase.\n example) klaytc -key='2ef07640fd8d3f568c23185799ee92e0154bf08ccfe5c509466d1d40baca3430'")
	}

	// setup default http client.
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr.MaxIdleConns = *httpMaxIdleConnsPtr
		tr.MaxIdleConnsPerHost = *httpMaxIdleConnsPtr
	}

	// for TC Selection
	if tcStr != "" {
		// Run tasks without connecting to the master.
		tcStrList = strings.Split(tcStr, ",")
	}

	gEndpoint = *gEndpointPtr

	nUserForSigned = *nUserForSignedPtr
	activeUserPercent = *activeUserPercentPtr
	coinbasePrivatekey = *keyPtr
	chargeValue = new(big.Int)
	chargeValue.Set(new(big.Int).Mul(big.NewInt(int64(chargeKLAYAmount)), big.NewInt(params.Wei)))

	fmt.Println("Arguments are set like the following:")
	fmt.Printf("- Target EndPoint = %v\n", gEndpoint)
	fmt.Printf("- activeUserPercent = %v\n", activeUserPercent)
	fmt.Printf("- coinbasePrivatekey = %v\n", coinbasePrivatekey)
	fmt.Printf("- tc = %v\n", tcStr)
}

func updateChainID() {
	fmt.Println("Updating ChainID from RPC")
	account.SetChainID(big.NewInt(1337))
}

func updateGasPrice(ctx context.Context, c *client.Client) {
	gasPrice = big.NewInt(750000000000)
	p, err := c.SuggestGasPrice(ctx)
	if err != nil {
		log.Fatalf("failed to get gas price")
	}
	account.SetGasPrice(p)
}

func updateBaseFee() {
	baseFee = big.NewInt(0)
	account.SetBaseFee(baseFee)
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
func initTCList() (taskSet []*ExtendedTask) {
	taskSet = append(taskSet, &ExtendedTask{
		Name:    "transferSignedTc",
		Weight:  10,
		Fn:      transferSignedTc.Run,
		Init:    transferSignedTc.Init,
		AccGrp:  accGrpForSignedTx,
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
	// Call initTCList to get all TC names
	taskSet := initTCList()

	var tcNames string
	for i, task := range taskSet {
		if i != 0 {
			tcNames += ","
		}
		tcNames += task.Name
	}

	initArgs(tcNames)

	// Create Cli pool
	gCli = Create(gEndpoint)

	// Update chainID
	updateChainID()

	// Update gasPrice
	updateGasPrice(context.Background(), gCli)

	// Update baseFee
	updateBaseFee()

	// Set coinbase & Create Test Account
	prepareAccounts()

	// Call initTCList again to actually define all TCs
	taskSet = initTCList()

	var filteredTask []*ExtendedTask

	println("Adding tasks")
	for _, task := range taskSet {
		if task.Name == "" {
			continue
		} else {
			flag := false
			for _, name := range tcStrList {
				if name == task.Name {
					flag = true
					break
				}
			}
			if flag {
				filteredTask = append(filteredTask, task)
				println("=> " + task.Name + " task is added.")
			}
		}
	}

	// Charge Accounts
	accGrp := make(map[common.Address]*account.Account)
	for _, task := range filteredTask {
		for _, acc := range task.AccGrp {
			_, exist := accGrp[acc.GetAddress()]
			if !exist {
				accGrp[acc.GetAddress()] = acc
			}
		}

	}

	if len(chargeValue.Bits()) != 0 {
		prepareTestAccountsAndContracts(accGrp)
	}

	// After charging accounts, cut the slice to the desired length, calculated by ActiveAccountPercent.
	for _, task := range filteredTask {
		if activeUserPercent > 100 {
			log.Fatalf("ActiveAccountPercent should be less than or equal to 100, but it is %v", activeUserPercent)
		}
		numActiveAccounts := len(task.AccGrp) * activeUserPercent / 100
		// Not to assign 0 account for some cases.
		if numActiveAccounts == 0 {
			numActiveAccounts = 1
		}
		task.AccGrp = task.AccGrp[:numActiveAccounts]
	}

	if len(filteredTask) == 0 {
		log.Fatal("No Tc is set. Please set TcList. \nExample argument) -tc='" + tcNames + "'")
	}

	println("Initializing tasks")
	var filteredBoomerTask []*boomer.Task
	for _, task := range filteredTask {
		task.Init(task.AccGrp, task.EndPint, gasPrice)
		filteredBoomerTask = append(filteredBoomerTask, &boomer.Task{task.Weight, task.Fn, task.Name})
		println("=> " + task.Name + " task is initialized.")
	}

	setRLimit(syscall.RLIMIT_NOFILE, 1024*400)

	// Locust Slave Run
	boomer.Run(filteredBoomerTask...)
	//boomer.Run(cpuHeavyTx)
}
