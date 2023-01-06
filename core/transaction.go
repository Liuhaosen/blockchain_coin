package core

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
)

const subsidy = 10 //默认挖矿得到的数量

type Transaction struct {
	ID   []byte     //交易序号
	Vin  []TXInput  //输入
	Vout []TXOutput //输出
}

type TXInput struct {
	Txid      []byte
	Vout      int    //金额
	ScriptSig string //签名,对输入的合法性做验证
}

type TXOutput struct {
	Value        int    //金额
	ScriptPubKey string //输出签名
}

//是否发币交易: true: 是发币交易, false: 否(是转账交易)
func (tx Transaction) IsCoinbase() bool {
	return len(tx.Vin) == 1 && len(tx.Vin[0].Txid) == 0 && tx.Vin[0].Vout == -1
}

//设置交易序号
func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx) //对整个交易数据进行编码
	if err != nil {
		log.Panic("encode transaction failed, err: ", err)
	}
	hash = sha256.Sum256(encoded.Bytes()) //计算哈希值
	tx.ID = hash[:]
}

func (in *TXInput) CanUnlockOutputWith(unlockingData string) bool {
	return in.ScriptSig == unlockingData
}

//验证该交易是否能解锁. 不正确就不能使用.
func (out *TXOutput) CanBeUnlockedWith(unlockingData string) bool {
	return out.ScriptPubKey == unlockingData
}

func NewTXOutput(value int, address string) *TXOutput {
	txo := &TXOutput{value, address}
	return txo
}

//创建一个发币奖励交易
func NewCoinbaseTX(to, data string) *Transaction {
	if data == "" {
		randData := make([]byte, 20)
		_, err := rand.Read(randData)
		if err != nil {
			log.Panic(err)
		}

		data = fmt.Sprintf("%x", randData)
	}

	txin := TXInput{[]byte{}, -1, data}                         //发币的输入是空的
	txout := NewTXOutput(subsidy, to)                           //默认挖矿数量
	tx := Transaction{nil, []TXInput{txin}, []TXOutput{*txout}} //创建交易
	tx.SetID()                                                  //设置交易号
	return &tx
}

//创建一个转账交易
func NewUTXOTransaction(from string, to string, amount int, bc *Blockchain) *Transaction {
	var inputs []TXInput   //输入
	var outputs []TXOutput //输出

	//1. 找到所有的输出交易, 看还有多少余额
	acc, validOutputs := bc.FindSpendableOutputs(from, amount)

	if acc < amount {
		log.Panic("ERROR: ", from, "余额不足")
	}

	//2. 生成输入
	for txid, outs := range validOutputs {
		//2.1. 循环所有输出的交易记录
		txID, err := hex.DecodeString(txid)
		if err != nil {
			log.Panic(err)
		}
		//2.2. 然后循环输出, 一笔笔变成新的交易的输入
		for _, out := range outs {
			input := TXInput{txID, out, from}
			inputs = append(inputs, input)
		}
	}

	//3. 生成输出
	outputs = append(outputs, TXOutput{amount, to})

	//3.1 找零,找零也是一个输出
	if acc > amount {
		outputs = append(outputs, *NewTXOutput(acc-amount, from)) // a change
	}

	//4. 生成新的交易记录
	tx := Transaction{nil, inputs, outputs}
	tx.SetID()
	return &tx
}
