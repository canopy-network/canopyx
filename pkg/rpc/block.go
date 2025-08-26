package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/canopy-network/canopyx/pkg/db/models/indexer"
)

// HeadBlock represents the response from the /v1/query/height endpoint.
type HeadBlock struct {
	Height uint64 `json:"height"`
}

// BlockByHeight represents the response from the /v1/query/block-by-height endpoint.
// NOTE: we can reduce the struct fields once we have a better understanding of the response and the indexing needs.
type BlockByHeight struct {
	BlockHeader struct {
		Height             uint64 `json:"height"`
		Hash               string `json:"hash"`
		NetworkID          int    `json:"networkID"`
		Time               int64  `json:"time"`
		TotalVDFIterations int    `json:"totalVDFIterations"`
		LastBlockHash      string `json:"lastBlockHash"`
		StateRoot          string `json:"stateRoot"`
		TransactionRoot    string `json:"transactionRoot"`
		ValidatorRoot      string `json:"validatorRoot"`
		NextValidatorRoot  string `json:"nextValidatorRoot"`
		ProposerAddress    string `json:"proposerAddress"`
		Vdf                struct {
			Proof      string `json:"proof"`
			Output     string `json:"output"`
			Iterations int    `json:"iterations"`
		} `json:"vdf"`
		LastQuorumCertificate struct {
			Header struct {
				Height          int    `json:"height"`
				CommitteeHeight int    `json:"committeeHeight"`
				Round           int    `json:"round"`
				Phase           string `json:"phase"`
				NetworkID       int    `json:"networkID"`
				ChainId         int    `json:"chainId"`
			} `json:"header"`
			BlockHash   string `json:"blockHash"`
			ResultsHash string `json:"resultsHash"`
			Results     struct {
				RewardRecipients struct {
					PaymentPercents []struct {
						Address  string `json:"address"`
						Percents int    `json:"percents"`
						ChainId  int    `json:"chainId"`
					} `json:"paymentPercents"`
				} `json:"rewardRecipients"`
				SlashRecipients struct {
				} `json:"slashRecipients"`
				Orders struct {
					LockOrders  interface{} `json:"lockOrders"`
					ResetOrders interface{} `json:"resetOrders"`
					CloseOrders interface{} `json:"closeOrders"`
				} `json:"orders"`
			} `json:"results"`
			ProposerKey string `json:"proposerKey"`
			Signature   struct {
				Signature string `json:"signature"`
				Bitmap    string `json:"bitmap"`
			} `json:"signature"`
		} `json:"lastQuorumCertificate"`
	} `json:"blockHeader"`
	Meta struct {
		Size int `json:"size"`
	} `json:"meta"`
}

// ToBlockModel converts a BlockByHeight to a BlockModel.
func (bbh *BlockByHeight) ToBlockModel() *indexer.Block {
	return &indexer.Block{
		Height:          bbh.BlockHeader.Height,
		Hash:            bbh.BlockHeader.Hash,
		Time:            time.UnixMicro(bbh.BlockHeader.Time),
		LastBlockHash:   bbh.BlockHeader.LastBlockHash,
		ProposerAddress: bbh.BlockHeader.ProposerAddress,
		Size:            bbh.Meta.Size,
	}
}

// ChainHead returns the height of the chain head.
func (c *HTTPClient) ChainHead(ctx context.Context) (uint64, error) {
	var resp HeadBlock
	err := c.doJSON(ctx, http.MethodPost, headPath, map[string]any{}, &resp)
	if err == nil && resp.Height > 0 {
		return resp.Height, nil
	}
	return uint64(0), fmt.Errorf("cannot probe head %v", err)
}

// BlockByHeight returns the block at the given height.
func (c *HTTPClient) BlockByHeight(ctx context.Context, h uint64) (*indexer.Block, error) {
	var out BlockByHeight
	if err := c.doJSON(ctx, http.MethodPost, blockByHeightPath, map[string]any{"height": h}, &out); err != nil {
		return nil, err
	}

	if out.BlockHeader.Height != h {
		// OR maybe just a bug?
		return nil, errors.New("block height is not ready yet (it comes empty from canopy)")
	}

	if out.Meta.Size == 0 {
		// OR maybe just a bug?
		return nil, errors.New("block size is zero (means that probably is not ready yet to be indexed)")
	}

	return out.ToBlockModel(), nil
}
