package account

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	blockchain "github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
	"strings"
)

var Erc20PerformanceABI = `[{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"value","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[],"name":"totalSupply","outputs":[{"name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"name":"sender","type":"address"},{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"addedValue","type":"uint256"}],"name":"increaseAllowance","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"name":"account","type":"address"},{"name":"amount","type":"uint256"}],"name":"mint","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":false,"inputs":[{"name":"account","type":"address"}],"name":"addMinter","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[],"name":"renounceMinter","outputs":[],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"subtractedValue","type":"uint256"}],"name":"decreaseAllowance","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":false,"inputs":[{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"isMinter","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"view","type":"function"},{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"},{"inputs":[],"payable":false,"stateMutability":"nonpayable","type":"constructor"},{"anonymous":false,"inputs":[{"indexed":true,"name":"account","type":"address"}],"name":"MinterAdded","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"account","type":"address"}],"name":"MinterRemoved","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"from","type":"address"},{"indexed":true,"name":"to","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Transfer","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"owner","type":"address"},{"indexed":true,"name":"spender","type":"address"},{"indexed":false,"name":"value","type":"uint256"}],"name":"Approval","type":"event"}]`
var erc20PerformanceByteCode = common.FromHex("60806040523480156200001157600080fd5b506200002c3362000053640100000000026401000000009004565b6200004c3364e8d4a51000620000bd640100000000026401000000009004565b5062000642565b620000778160036200019964010000000002620013cc179091906401000000009004565b8073ffffffffffffffffffffffffffffffffffffffff167f6ae172837ea30b801fbfcdd4108aa1d5bf8ff775444fd70256b44e6bf3dfc3f660405160405180910390a250565b6000620000d93362000288640100000000026401000000009004565b151562000174576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001807f4d696e746572526f6c653a2063616c6c657220646f6573206e6f74206861766581526020017f20746865204d696e74657220726f6c650000000000000000000000000000000081525060400191505060405180910390fd5b6200018f8383620002b5640100000000026401000000009004565b6001905092915050565b620001b4828262000493640100000000026401000000009004565b1515156200022a576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f526f6c65733a206163636f756e7420616c72656164792068617320726f6c650081525060200191505060405180910390fd5b60018260000160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055505050565b6000620002ae8260036200049364010000000002620012a9179091906401000000009004565b9050919050565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156200035b576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f45524332303a206d696e7420746f20746865207a65726f20616464726573730081525060200191505060405180910390fd5b6200038081600254620005b76401000000000262000fae179091906401000000009004565b600281905550620003e7816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054620005b76401000000000262000fae179091906401000000009004565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a35050565b60008073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff161415151562000560576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260228152602001807f526f6c65733a206163636f756e7420697320746865207a65726f20616464726581526020017f737300000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b8260000160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff16905092915050565b600080828401905083811015151562000638576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b6115d780620006526000396000f3006080604052600436106100ba576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff168063095ea7b3146100bf57806318160ddd1461012457806323b872dd1461014f57806339509351146101d457806340c10f191461023957806370a082311461029e578063983b2d56146102f55780639865027514610338578063a457c2d71461034f578063a9059cbb146103b4578063aa271e1a14610419578063dd62ed3e14610474575b600080fd5b3480156100cb57600080fd5b5061010a600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506104eb565b604051808215151515815260200191505060405180910390f35b34801561013057600080fd5b50610139610502565b6040518082815260200191505060405180910390f35b34801561015b57600080fd5b506101ba600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061050c565b604051808215151515815260200191505060405180910390f35b3480156101e057600080fd5b5061021f600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506105bd565b604051808215151515815260200191505060405180910390f35b34801561024557600080fd5b50610284600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610662565b604051808215151515815260200191505060405180910390f35b3480156102aa57600080fd5b506102df600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061071b565b6040518082815260200191505060405180910390f35b34801561030157600080fd5b50610336600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610763565b005b34801561034457600080fd5b5061034d610812565b005b34801561035b57600080fd5b5061039a600480360381019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019092919050505061081d565b604051808215151515815260200191505060405180910390f35b3480156103c057600080fd5b506103ff600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506108c2565b604051808215151515815260200191505060405180910390f35b34801561042557600080fd5b5061045a600480360381019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506108d9565b604051808215151515815260200191505060405180910390f35b34801561048057600080fd5b506104d5600480360381019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506108f6565b6040518082815260200191505060405180910390f35b60006104f833848461097d565b6001905092915050565b6000600254905090565b6000610519848484610bfe565b6105b284336105ad85600160008a73ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610f2490919063ffffffff16565b61097d565b600190509392505050565b6000610658338461065385600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fae90919063ffffffff16565b61097d565b6001905092915050565b600061066d336108d9565b1515610707576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001807f4d696e746572526f6c653a2063616c6c657220646f6573206e6f74206861766581526020017f20746865204d696e74657220726f6c650000000000000000000000000000000081525060400191505060405180910390fd5b6107118383611038565b6001905092915050565b60008060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020549050919050565b61076c336108d9565b1515610806576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001807f4d696e746572526f6c653a2063616c6c657220646f6573206e6f74206861766581526020017f20746865204d696e74657220726f6c650000000000000000000000000000000081525060400191505060405180910390fd5b61080f816111f5565b50565b61081b3361124f565b565b60006108b833846108b385600160003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008973ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610f2490919063ffffffff16565b61097d565b6001905092915050565b60006108cf338484610bfe565b6001905092915050565b60006108ef8260036112a990919063ffffffff16565b9050919050565b6000600160008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054905092915050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610a48576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260248152602001807f45524332303a20617070726f76652066726f6d20746865207a65726f2061646481526020017f726573730000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610b13576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260228152602001807f45524332303a20617070726f766520746f20746865207a65726f20616464726581526020017f737300000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b80600160008573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060008473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925836040518082815260200191505060405180910390a3505050565b600073ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff1614151515610cc9576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260258152602001807f45524332303a207472616e736665722066726f6d20746865207a65726f20616481526020017f647265737300000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515610d94576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260238152602001807f45524332303a207472616e7366657220746f20746865207a65726f206164647281526020017f657373000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b610de5816000808673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610f2490919063ffffffff16565b6000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002081905550610e78816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fae90919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff168373ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a3505050565b600080838311151515610f9f576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f536166654d6174683a207375627472616374696f6e206f766572666c6f77000081525060200191505060405180910390fd5b82840390508091505092915050565b600080828401905083811015151561102e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff16141515156110dd576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f45524332303a206d696e7420746f20746865207a65726f20616464726573730081525060200191505060405180910390fd5b6110f281600254610fae90919063ffffffff16565b600281905550611149816000808573ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002054610fae90919063ffffffff16565b6000808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055508173ffffffffffffffffffffffffffffffffffffffff16600073ffffffffffffffffffffffffffffffffffffffff167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040518082815260200191505060405180910390a35050565b6112098160036113cc90919063ffffffff16565b8073ffffffffffffffffffffffffffffffffffffffff167f6ae172837ea30b801fbfcdd4108aa1d5bf8ff775444fd70256b44e6bf3dfc3f660405160405180910390a250565b6112638160036114a990919063ffffffff16565b8073ffffffffffffffffffffffffffffffffffffffff167fe94479a9f7e1952cc78f2d6baab678adc1b772d936c6583def489e524cb6669260405160405180910390a250565b60008073ffffffffffffffffffffffffffffffffffffffff168273ffffffffffffffffffffffffffffffffffffffff1614151515611375576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260228152602001807f526f6c65733a206163636f756e7420697320746865207a65726f20616464726581526020017f737300000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b8260000160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060009054906101000a900460ff16905092915050565b6113d682826112a9565b15151561144b576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601f8152602001807f526f6c65733a206163636f756e7420616c72656164792068617320726f6c650081525060200191505060405180910390fd5b60018260000160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff0219169083151502179055505050565b6114b382826112a9565b151561154d576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260218152602001807f526f6c65733a206163636f756e7420646f6573206e6f74206861766520726f6c81526020017f650000000000000000000000000000000000000000000000000000000000000081525060400191505060405180910390fd5b60008260000160008373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200190815260200160002060006101000a81548160ff02191690831515021790555050505600a165627a7a72305820577de674f02c621a82595da1d61a932e3fd2a3286a9a4e9dbf48df7002e9b5010029")

func (self *Account) DeployERC20(c *ethclient.Client, to *Account, value *big.Int, humanReadable bool) (common.Address, *types.Transaction, *big.Int, error) {
	ctx := context.Background() //context.WithTimeout(context.Background(), 100*time.Second)

	self.mutex.Lock()
	defer self.mutex.Unlock()

	nonce := self.GetNonce(c)
	//if nonce != 0 {
	//	fmt.Println("Contract seems to already have been deployed!", "nonce", nonce)
	//	return common.Address{}, nil, nil, AlreadyDeployedErr
	//}

	gaslimit := uint64(10000000)
	if humanReadable {
		gaslimit = uint64(4100000000)
	}

	contractABI := Erc20PerformanceABI
	parsed, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		fmt.Println("Error while parsing contractABI", "err", err)
	}
	byteCode := erc20PerformanceByteCode

	txOpts := &bind.TransactOpts{
		From: self.address, Nonce: big.NewInt(int64(nonce)),
		Signer: func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
			if address != self.address {
				return nil, errors.New("not authorized to sign this account")
			}
			return types.SignTx(tx, types.NewEIP155Signer(chainID), self.privateKey[0])
		}, Value: common.Big0,
		GasPrice: gasPrice, GasLimit: gaslimit, Context: ctx}
	contractAddr, contractTx, _, err := bind.DeployContract(txOpts, parsed, byteCode, c)
	if err != nil {
		if strings.EqualFold(err.Error(), blockchain.ErrNonceTooLow.Error()) || strings.EqualFold(err.Error(), txpool.ErrReplaceUnderpriced.Error()) {
			log.Printf("Failed to DeployContract tx: %v", err)
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
			fmt.Printf("Account(%v) nonce is added to %v\n", self.GetAddress().String(), nonce+1)
			self.nonce++
		} else {
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
		}
		return contractAddr, contractTx, nil, err
	}

	self.nonce++
	return contractAddr, contractTx, gasPrice, nil
}

func (self *Account) TransferERC20(initialCharge bool, c *ethclient.Client, tokenContractAddr common.Address, tokenRecipient *Account, value *big.Int) (*types.Transaction, *big.Int, error) {
	ctx := context.Background() //context.WithTimeout(context.Background(), 100*time.Second)
	self.mutex.Lock()
	defer self.mutex.Unlock()

	log.Printf("tokenAddr: %s, recioient: %s, value: %s", tokenContractAddr.Hex(), tokenRecipient.address, value.Text(10))
	var nonce uint64
	if initialCharge {
		nonce = self.GetNonceFromBlock(c)
	} else {
		nonce = self.GetNonce(c)
	}
	log.Printf("self addr: %s, self nonce: %d, self key: %s", self.address, nonce, self.GetKey())

	abiStr := Erc20PerformanceABI

	contractAbi, err := abi.JSON(strings.NewReader(abiStr))
	if err != nil {
		log.Fatalf("failed to abi.JSON: %v", err)
	}
	data, err := contractAbi.Pack("transfer", tokenRecipient.address, value)
	if err != nil {
		log.Fatalf("failed to abi.Pack: %v", err)
	}
	fmt.Println("gas price", gasPrice)
	tx := types.NewTransaction(nonce, tokenContractAddr, common.Big0, uint64(5000000), gasPrice, data)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), self.GetKey())
	if err != nil {
		log.Fatal(err)
	}

	err = c.SendTransaction(ctx, signedTx)
	if err != nil {
		if strings.EqualFold(err.Error(), blockchain.ErrNonceTooLow.Error()) || strings.EqualFold(err.Error(), txpool.ErrReplaceUnderpriced.Error()) {
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
			fmt.Printf("Account(%v) nonce is added to %v\n", self.GetAddress().String(), nonce+1)
			self.nonce++
		} else {
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
		}
		return signedTx, gasPrice, err
	}

	self.nonce++
	return signedTx, gasPrice, nil
}

// self = deployer
func (self *Account) Mint(initialCharge bool, c *ethclient.Client, tokenContractAddr common.Address, tokenRecipient *Account, value *big.Int) (*types.Transaction, *big.Int, error) {
	log.Printf("Minting start")
	ctx := context.Background() //context.WithTimeout(context.Background(), 100*time.Second)
	self.mutex.Lock()
	self.mutex.Unlock()

	var nonce uint64
	if initialCharge {
		nonce = self.GetNonceFromBlock(c)
	} else {
		nonce = self.GetNonce(c)
	}

	abiStr := Erc20PerformanceABI

	contractAbi, err := abi.JSON(strings.NewReader(abiStr))
	if err != nil {
		log.Fatalf("failed to abi.JSON: %v", err)
	}
	data, err := contractAbi.Pack("mint", self.address, value)
	if err != nil {
		log.Fatalf("failed to abi.Pack: %v", err)
	}

	tx := types.NewTransaction(nonce, tokenContractAddr, value, uint64(5000000), gasPrice, data)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), self.GetKey())
	if err != nil {
		log.Fatal(err)
	}

	err = c.SendTransaction(ctx, signedTx)
	if err != nil {
		if err.Error() == blockchain.ErrNonceTooLow.Error() || err.Error() == txpool.ErrReplaceUnderpriced.Error() {
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
			fmt.Printf("Account(%v) nonce is added to %v\n", self.GetAddress().String(), nonce+1)
			self.nonce++
		} else {
			fmt.Printf("Account(%v) nonce(%v) : Failed to sendTrasnaction: %v\n", self.GetAddress().String(), nonce, err)
		}
		return signedTx, gasPrice, err
	}

	self.nonce++

	data, err = contractAbi.Pack("balanceOf", tokenRecipient.address)
	if err != nil {
		log.Fatal(err)
	}

	msg := ethereum.CallMsg{
		To:   &tokenContractAddr,
		Data: data,
	}

	result, err := c.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Fatalf("Failed to call contract: %v", err)
	}

	balanceResult, err := contractAbi.Unpack("balanceOf", result)
	if err != nil {
		log.Fatalf("Failed to unpack data: %v", err)
	}

	if len(balanceResult) == 0 {
		log.Fatalf("Invalid result length")
	}

	finalBalance := balanceResult[0].(*big.Int)
	fmt.Printf("Contract Balance: %s\n", finalBalance.String())

	return signedTx, gasPrice, nil
}
