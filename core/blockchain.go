package core

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/boltdb/bolt"
)

type Blockchain struct {
	tip []byte //存放l哈希值
	db  *bolt.DB
}

type BlockchainIterator struct {
	currentHash []byte
	db          *bolt.DB
}

const dbFile = "blockchain.db"
const blocksBucket = "blocks"
const genesisCoinbaseData = "The Times 03/Jan/2009 Chancellor on brink of second bailout for banks"

//初始化区块链
func NewBlockchain(address string) *Blockchain {
	if !dbExists(dbFile) {
		fmt.Println("No existing blockchain found, create one first")
		os.Exit(1)
	}

	var tip []byte
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Panic("open bolt db file failed, err :", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		tip = b.Get([]byte("l"))
		return nil
	})

	if err != nil {
		log.Panic("更新数据库失败, 错误: ", err)
	}
	bc := &Blockchain{tip, db}
	return bc
}

//创建一个新的区块链数据库
func CreateBlockchain(address string) *Blockchain {
	if dbExists(dbFile) {
		fmt.Println("blockchain already exists")
		os.Exit(1)
	}

	var tip []byte
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Panic("dbfile open failed", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		cbtx := NewCoinbaseTX(address, genesisCoinbaseData)

		genesis := NewGenesisBlock(cbtx)

		b, err := tx.CreateBucket([]byte(blocksBucket))
		if err != nil {
			log.Panic("create bucket failed, err: ", err)
		}

		err = b.Put(genesis.Hash, genesis.Serialize())
		if err != nil {
			log.Panic("Save genesis failed, err: ", err)
		}

		err = b.Put([]byte("l"), genesis.Hash)
		if err != nil {
			log.Panic("Save leader hash failed, err: ", err)
		}
		tip = genesis.Hash
		return nil
	})

	if err != nil {
		log.Panic("save genesis data failed, err:", err)
	}

	return &Blockchain{tip, db}
}

//使用提供的交易记录挖掘新块
func (bc *Blockchain) MineBlock(transactions []*Transaction) {
	var lastHash []byte

	err := bc.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		lastHash = b.Get([]byte("l"))
		return nil
	})

	if err != nil {
		log.Panic("get lastHash failed, err: ", err)
	}
	newBlock := NewBlock(transactions, lastHash)

	err = bc.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		err := b.Put(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic("save newblock failed, err:", err)
		}
		err = b.Put([]byte("l"), newBlock.Hash)
		if err != nil {
			log.Panic("save leader hash failed, err:", err)
		}

		bc.tip = newBlock.Hash
		return nil
	})
	if err != nil {
		log.Panic("save new block failed, err:", err)
	}
}

/* //添加新区块
func (bc *Blockchain) AddBlock(data string) {
	var lastHash []byte

	err := bc.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		lastHash = b.Get([]byte("l"))
		return nil
	})

	if err != nil {
		log.Panic("获取数据库值失败, 错误:", err)
	}

	newBlock := NewBlock(data, lastHash)

	err = bc.Db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		err := b.Put(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic("存储新block失败, 错误:", err)
		}

		err = b.Put([]byte("l"), newBlock.Hash)
		if err != nil {
			log.Panic("存储lhash失败,错误:", err)
		}
		bc.tip = newBlock.Hash
		return nil
	})

	if err != nil {
		log.Panic(err)
	}
}
*/
func (bci *BlockchainIterator) Next() *Block {
	var block *Block
	err := bci.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encodedBlock := b.Get(bci.currentHash)
		block = DeserializeBlock(encodedBlock)
		return nil
	})
	if err != nil {
		log.Panic("获取block失败, 错误:", err)
	}

	bci.currentHash = block.PrevBlockHash
	return block
}

func (bc *Blockchain) Iterator() *BlockchainIterator {
	bci := &BlockchainIterator{bc.tip, bc.db}
	return bci
}

//返回一个没花出去的交易列表
func (bc *Blockchain) FindUnspentTransactions(address string) []Transaction {
	var unspentTXs []Transaction        //未转账的交易记录
	spentTXOs := make(map[string][]int) //保存已转账的输出
	bci := bc.Iterator()                //迭代器

	for {
		block := bci.Next() //迭代, 挨个区块查看
		//1. 把所有交易拿出来开始循环
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)

		Outputs:
			for outIdx, out := range tx.Vout {
				// Was the output spent?
				//如果这个交易id不等于空
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						//如果已转账的输出 == 交易记录的输出, 那就跳过,接着执行Outputs, 直到找到没转账的, 放入unspentTXs
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				if out.CanBeUnlockedWith(address) {
					unspentTXs = append(unspentTXs, *tx)
				}
			}

			//1.2 如果是转账交易
			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					//判定是否属于自己
					if in.CanUnlockOutputWith(address) {
						inTxID := hex.EncodeToString(in.Txid)
						//凡是有输入的交易记录, 都证明是花出去了. 所以要找没花出去的, 就要先找花出去的. 然后从记录里直接剔除就行.
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout) //记录到已转账的输出集合里
					}
				}
			}
		}

		if len(block.PrevBlockHash) == 0 {
			break
		}
	}

	return unspentTXs
}

func dbExists(dbFile string) bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}
	return true
}

//找到可以消费的输出
func (bc *Blockchain) FindSpendableOutputs(from string, amount int) (int, map[string][]int) {
	unspentOutputs := make(map[string][]int)       //未转账的输出
	unspentTXs := bc.FindUnspentTransactions(from) //未转账的交易记录
	accumulated := 0                               //累计金额

Work:
	//1. 循环没花完的交易记录, 保存没花完的输出, 并计算累计金额
	for _, tx := range unspentTXs {
		txID := hex.EncodeToString(tx.ID)

		for outIdx, out := range tx.Vout {
			//1.1 如果这笔输出的金额小于要转账的金额, 那么就累加余额
			if out.CanBeUnlockedWith(from) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)

				//1.2 如果余额已经大于要转账的金额, 直接退出即可
				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOutputs
}

//找到属于某个地址的, 没转账的交易记录的输出
func (bc *Blockchain) FindUTXO(address string) []TXOutput {
	var UTXOs []TXOutput
	unspentTransactions := bc.FindUnspentTransactions(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Vout {
			if out.CanBeUnlockedWith(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}
	return UTXOs
}
