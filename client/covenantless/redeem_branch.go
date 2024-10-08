package covenantless

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/ark-network/ark/client/utils"
	"github.com/ark-network/ark/common/bitcointree"
	"github.com/ark-network/ark/common/tree"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/txscript"
	"github.com/urfave/cli/v2"
)

type redeemBranch struct {
	vtxo     *vtxo
	branch   []*psbt.Packet
	lifetime time.Duration
	explorer utils.Explorer
}

func newRedeemBranch(
	explorer utils.Explorer,
	congestionTree tree.CongestionTree, vtxo vtxo,
) (*redeemBranch, error) {
	_, seconds, err := findSweepClosure(congestionTree)
	if err != nil {
		return nil, err
	}

	lifetime, err := time.ParseDuration(fmt.Sprintf("%ds", seconds))
	if err != nil {
		return nil, err
	}

	nodes, err := congestionTree.Branch(vtxo.txid)
	if err != nil {
		return nil, err
	}

	branch := make([]*psbt.Packet, 0, len(nodes))
	for _, node := range nodes {
		ptx, err := psbt.NewFromRawBytes(strings.NewReader(node.Tx), true)
		if err != nil {
			return nil, err
		}
		branch = append(branch, ptx)
	}

	return &redeemBranch{
		vtxo:     &vtxo,
		branch:   branch,
		lifetime: lifetime,
		explorer: explorer,
	}, nil
}

// RedeemPath returns the list of transactions to broadcast in order to access the vtxo output
func (r *redeemBranch) redeemPath() ([]string, error) {
	transactions := make([]string, 0, len(r.branch))

	offchainPath, err := r.offchainPath()
	if err != nil {
		return nil, err
	}

	for _, ptx := range offchainPath {
		firstInput := ptx.Inputs[0]
		if len(firstInput.TaprootKeySpendSig) == 0 {
			return nil, fmt.Errorf("missing taproot key spend signature")
		}

		var witness bytes.Buffer

		if err := psbt.WriteTxWitness(&witness, [][]byte{firstInput.TaprootKeySpendSig}); err != nil {
			return nil, err
		}

		ptx.Inputs[0].FinalScriptWitness = witness.Bytes()

		extracted, err := psbt.Extract(ptx)
		if err != nil {
			return nil, err
		}

		var txBytes bytes.Buffer

		if err := extracted.Serialize(&txBytes); err != nil {
			return nil, err
		}

		transactions = append(transactions, hex.EncodeToString(txBytes.Bytes()))
	}

	return transactions, nil
}

func (r *redeemBranch) expireAt(*cli.Context) (*time.Time, error) {
	lastKnownBlocktime := int64(0)

	confirmed, blocktime, _ := r.explorer.GetTxBlocktime(r.vtxo.poolTxid)

	if confirmed {
		lastKnownBlocktime = blocktime
	} else {
		expirationFromNow := time.Now().Add(time.Minute).Add(r.lifetime)
		return &expirationFromNow, nil
	}

	for _, ptx := range r.branch {
		txid := ptx.UnsignedTx.TxHash().String()

		confirmed, blocktime, err := r.explorer.GetTxBlocktime(txid)
		if err != nil {
			break
		}

		if confirmed {
			lastKnownBlocktime = blocktime
			continue
		}

		break
	}

	t := time.Unix(lastKnownBlocktime, 0).Add(r.lifetime)
	return &t, nil
}

// offchainPath checks for transactions of the branch onchain and returns only the offchain part
func (r *redeemBranch) offchainPath() ([]*psbt.Packet, error) {
	offchainPath := append([]*psbt.Packet{}, r.branch...)

	for i := len(r.branch) - 1; i >= 0; i-- {
		ptx := r.branch[i]
		txHash := ptx.UnsignedTx.TxHash().String()

		if _, err := r.explorer.GetTxHex(txHash); err != nil {
			continue
		}

		// if no error, the tx exists onchain, so we can remove it (+ the parents) from the branch
		if i == len(r.branch)-1 {
			offchainPath = []*psbt.Packet{}
		} else {
			offchainPath = r.branch[i+1:]
		}

		break
	}

	return offchainPath, nil
}

func findSweepClosure(
	congestionTree tree.CongestionTree,
) (*txscript.TapLeaf, uint, error) {
	root, err := congestionTree.Root()
	if err != nil {
		return nil, 0, err
	}

	// find the sweep closure
	tx, err := psbt.NewFromRawBytes(strings.NewReader(root.Tx), true)
	if err != nil {
		fmt.Println("find sweep closure error")
		return nil, 0, err
	}

	var seconds uint
	var sweepClosure *txscript.TapLeaf
	for _, tapLeaf := range tx.Inputs[0].TaprootLeafScript {
		closure := &bitcointree.CSVSigClosure{}
		valid, err := closure.Decode(tapLeaf.Script)
		if err != nil {
			continue
		}

		if valid && closure.Seconds > seconds {
			seconds = closure.Seconds
			leaf := txscript.NewBaseTapLeaf(tapLeaf.Script)
			sweepClosure = &leaf
		}
	}

	if sweepClosure == nil {
		return nil, 0, fmt.Errorf("sweep closure not found")
	}

	return sweepClosure, seconds, nil
}
