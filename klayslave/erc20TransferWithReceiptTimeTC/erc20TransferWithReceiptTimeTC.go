package erc20TransferWithReceiptTimeTC

import (
	"fmt"
	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/clipool"
	"github.com/klaytn/klaytn-load-tester/klayslave/receiptChecker"
	"github.com/klaytn/klaytn-load-tester/klayslave/task"
	"log"
	"math/big"
	"math/rand"
	"time"

	"github.com/myzhan/boomer"
)

const Name = "erc20TransferTCWithReceiptTime"

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
	receipt              receiptChecker.ReceiptChecker
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

	if params.ReceiptChecker == nil {
		log.Fatal("Receipt checker not set")
	}
	receipt = params.ReceiptChecker
	go receipt.Check()
}

func Run() {
	cli := cliPool.Alloc().(*client.Client)

	from := accGrp[rand.Int()%nAcc]
	to := accGrp[rand.Int()%nAcc]
	value := big.NewInt(int64(rand.Int() % 3))

	start := time.Now()
	tx, _, err := from.TransferERC20(false, cli, SmartContractAccount.GetAddress(), to, value)
	if err != nil {
		elapsed := time.Since(start)
		elapsedMillis := elapsed.Milliseconds()
		if elapsedMillis == 0 {
			fmt.Printf("found zero elapsed ms. %d ns\n", elapsed.Nanoseconds())
		}

		boomer.RecordFailure("http", Name+"_fail", elapsedMillis, err.Error())
	}
	receipt.PushTransaction(tx.Hash().Hex())
}
