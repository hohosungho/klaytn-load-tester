package transferSignedTc

import (
	"fmt"
	"github.com/klaytn/klaytn-load-tester/klayslave/task"
	"log"
	"math/big"
	"math/rand"
	"time"

	client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"github.com/klaytn/klaytn-load-tester/klayslave/clipool"
	"github.com/myzhan/boomer"
)

const Name = "transferSignedTx"

var (
	endPoint          string
	reportName        string
	failureReportName string
	fromActiveUser    int
	toActiveUser      int
	accGrp            []*account.Account
	cliPool           clipool.ClientPool
	gasPrice          *big.Int

	// multinode tester
	transferedValue *big.Int
	expectedFee     *big.Int

	fromAccount     *account.Account
	prevBalanceFrom *big.Int

	toAccount     *account.Account
	prevBalanceTo *big.Int
)

func Init(params task.Params) {
	gasPrice = params.GasPrice

	endPoint = params.Endpoint
	reportName = "signedtransfer"
	if !params.AggregateTcName {
		reportName += " to " + endPoint
	}
	failureReportName = reportName + "_fail"

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
	fromActiveUser = params.ActiveFromUsers
	toActiveUser = params.ActiveToUsers
}

func Run() {
	cli := cliPool.Alloc().(*client.Client)

	from := accGrp[rand.Int()%fromActiveUser]
	to := accGrp[rand.Int()%toActiveUser]
	value := big.NewInt(int64(rand.Intn(1000) % 3))

	start := time.Now()
	_, _, err := from.TransferSignedTx(cli, to, value)
	elapsed := time.Since(start)
	elapsedMillis := elapsed.Milliseconds()
	if elapsedMillis == 0 {
		fmt.Printf("found zero elapsed ms. %d ns\n", elapsed.Nanoseconds())
	}

	if err == nil {
		boomer.RecordSuccess("http", reportName, elapsedMillis, int64(10))
		cliPool.Free(cli)
	} else {
		boomer.RecordFailure("http", failureReportName, elapsedMillis, err.Error())
	}
}
