package erc20TransferTC

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/clipool"
	"github.com/klaytn/klaytn-load-tester/klayslave/task"
	"github.com/myzhan/boomer"
	"log"
	"math/big"
	"math/rand"
	"sync"
	"time"
)

const Name = "erc20TransferTC"

var (
	endPoint string
	nAcc     int
	accGrp   []*account.Account
	cliPool  clipool.ClientPool
	gasPrice *big.Int

	// multinode tester
	transferedValue *big.Int
	expectedFee     *big.Int

	fromAccount     *account.Account
	prevBalanceFrom *big.Int

	toAccount     *account.Account
	prevBalanceTo *big.Int

	SmartContractAccount *account.Account
	txhash               = make(map[string]struct{})
	txhashLock           = sync.Mutex{}
)

func Init(params task.Params) {
	gasPrice = params.GasPrice

	endPoint = params.Endpoint

	cliCreate := func() interface{} {
		c, err := client.Dial(endPoint)
		if err != nil {
			log.Fatalf("Failed to connect RPC: %v", err)
		}
		return c
	}

	cliPool.Init(20, 300, cliCreate)

	for _, acc := range params.AccGrp {
		accGrp = append(accGrp, acc)
	}

	nAcc = len(accGrp)
}

func Run() {
	cli := cliPool.Alloc().(*client.Client)

	from := accGrp[rand.Int()%nAcc]
	to := accGrp[rand.Int()%nAcc]
	value := big.NewInt(int64(rand.Int() % 3))

	start := boomer.Now()
	tx, _, err := from.TransferERC20(false, cli, SmartContractAccount.GetAddress(), to, value)
	elapsed := boomer.Now() - start

	if err == nil {
		boomer.Events.Publish("request_success", "http", Name+" to "+endPoint, elapsed, int64(10))
		txhashLock.Lock()
		txhash[tx.Hash().String()] = struct{}{}
		txhashLock.Unlock()
		cliPool.Free(cli)
	} else {
		boomer.Events.Publish("request_failure", "http", Name+" to "+endPoint, elapsed, err.Error())
	}
}

func Stop() {
	log.Println("starting stop method")
	time.Sleep(30 * time.Second) //waiting
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()
	fail := 0
	errs := 0
	total := 0
	txhashLock.Lock()
	txhashs := txhash
	txhashLock.Unlock()
	hashes := getThousand(txhashs)

	for _, hash := range hashes {
		log.Println("confirming hash: " + hash)

		total++
		cli := cliPool.Alloc().(*client.Client)
		receipt, err := cli.TransactionReceipt(ctx, common.HexToHash(hash))
		if err != nil {
			errs++
		} else {
			if receipt.Status == 0 {
				fail++
			}
		}
		cliPool.Free(cli)
	}
	log.Printf("Total: %d, Failed: %d: errors: %d\n", total, fail, errs)
}

func getThousand(m map[string]struct{}) []string {
	h := make([]string, 1000)

	i := 0
	for k := range m {
		h[i] = k
		i++
		if i == 1000 {
			break
		}
	}
	return h
}
