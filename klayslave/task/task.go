package task

import (
	"github.com/klaytn/klaytn-load-tester/klayslave/account"
	"math/big"
)

type Params struct {
	AccGrp          []*account.Account
	Endpoint        string
	GasPrice        *big.Int
	AggregateTcName bool
	ActiveFromUsers int
	ActiveToUsers   int
}

type ExtendedTask struct {
	Name    string
	Weight  int
	Fn      func()
	Init    func(params Params)
	Stop    func()
	AccGrp  []*account.Account
	EndPint string
}
