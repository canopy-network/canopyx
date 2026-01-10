package rpc

import (
	"context"
	"fmt"
	"sync"

	"github.com/canopy-network/canopy/fsm"
	"github.com/canopy-network/canopy/lib"
)

type blobFactory struct {
	height uint64
	blobs  *fsm.IndexerBlobs
}

// NewBlobFactory builds an RPC factory that serves data from a pre-fetched blob.
func NewBlobFactory(height uint64, blobs *fsm.IndexerBlobs) Factory {
	return &blobFactory{height: height, blobs: blobs}
}

func (f *blobFactory) NewClient(_ []string) Client {
	return NewBlobClient(f.height, f.blobs)
}

// BlobClient implements RPC calls backed by a pre-fetched indexer blob.
type BlobClient struct {
	height uint64
	blobs  *fsm.IndexerBlobs

	currentOnce  sync.Once
	previousOnce sync.Once
	currentData  *blobData
	previousData *blobData
	currentErr   error
	previousErr  error
}

type blobData struct {
	block                *lib.BlockResult
	accounts             []*fsm.Account
	pools                []*fsm.Pool
	validators           []*fsm.Validator
	dexPrices            []*lib.DexPrice
	nonSigners           []*fsm.NonSigner
	doubleSigners        []*lib.DoubleSigner
	orders               []*lib.SellOrder
	params               *fsm.Params
	dexBatches           []*lib.DexBatch
	nextDexBatches       []*lib.DexBatch
	committeesData       []*lib.CommitteeData
	subsidizedCommittees []uint64
	retiredCommittees    []uint64
	supply               *fsm.Supply
}

// NewBlobClient returns a BlobClient for the given height and blob payload.
func NewBlobClient(height uint64, blobs *fsm.IndexerBlobs) *BlobClient {
	return &BlobClient{height: height, blobs: blobs}
}

func (c *BlobClient) ChainHead(_ context.Context) (uint64, error) {
	if c.height == 0 {
		return 0, fmt.Errorf("blob client has no height")
	}
	return c.height, nil
}

func (c *BlobClient) Blob(_ context.Context, _ uint64) (*fsm.IndexerBlobs, error) {
	if c.blobs == nil {
		return nil, fmt.Errorf("blob client has no blobs")
	}
	return c.blobs, nil
}

func (c *BlobClient) BlockByHeight(_ context.Context, height uint64) (*lib.BlockResult, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.block == nil {
		return nil, fmt.Errorf("block missing for height %d", height)
	}
	return data.block, nil
}

func (c *BlobClient) TxsByHeight(ctx context.Context, height uint64) ([]*lib.TxResult, error) {
	block, err := c.BlockByHeight(ctx, height)
	if err != nil {
		return nil, err
	}
	if block.Transactions == nil {
		return []*lib.TxResult{}, nil
	}
	return block.Transactions, nil
}

func (c *BlobClient) EventsByHeight(ctx context.Context, height uint64) ([]*lib.Event, error) {
	block, err := c.BlockByHeight(ctx, height)
	if err != nil {
		return nil, err
	}
	if block.Events == nil {
		return []*lib.Event{}, nil
	}
	return block.Events, nil
}

func (c *BlobClient) AccountsByHeight(_ context.Context, height uint64) ([]*fsm.Account, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.accounts == nil {
		return []*fsm.Account{}, nil
	}
	return data.accounts, nil
}

func (c *BlobClient) ValidatorsByHeight(_ context.Context, height uint64) ([]*fsm.Validator, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.validators == nil {
		return []*fsm.Validator{}, nil
	}
	return data.validators, nil
}

func (c *BlobClient) OrdersByHeight(_ context.Context, height uint64) ([]*lib.SellOrder, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.orders == nil {
		return []*lib.SellOrder{}, nil
	}
	return data.orders, nil
}

func (c *BlobClient) DexPricesByHeight(_ context.Context, height uint64) ([]*lib.DexPrice, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.dexPrices == nil {
		return []*lib.DexPrice{}, nil
	}
	return data.dexPrices, nil
}

func (c *BlobClient) PoolsByHeight(_ context.Context, height uint64) ([]*fsm.Pool, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.pools == nil {
		return []*fsm.Pool{}, nil
	}
	return data.pools, nil
}

func (c *BlobClient) AllDexBatchesByHeight(_ context.Context, height uint64) ([]*lib.DexBatch, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.dexBatches == nil {
		return []*lib.DexBatch{}, nil
	}
	return data.dexBatches, nil
}

func (c *BlobClient) AllNextDexBatchesByHeight(_ context.Context, height uint64) ([]*lib.DexBatch, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.nextDexBatches == nil {
		return []*lib.DexBatch{}, nil
	}
	return data.nextDexBatches, nil
}

func (c *BlobClient) AllParamsByHeight(_ context.Context, height uint64) (*fsm.Params, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.params == nil {
		return &fsm.Params{}, nil
	}
	return data.params, nil
}

func (c *BlobClient) ValParamsByHeight(ctx context.Context, height uint64) (*fsm.ValidatorParams, error) {
	params, err := c.AllParamsByHeight(ctx, height)
	if err != nil {
		return nil, err
	}
	if params.Validator == nil {
		return &fsm.ValidatorParams{}, nil
	}
	return params.Validator, nil
}

func (c *BlobClient) NonSignersByHeight(_ context.Context, height uint64) ([]*fsm.NonSigner, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.nonSigners == nil {
		return []*fsm.NonSigner{}, nil
	}
	return data.nonSigners, nil
}

func (c *BlobClient) DoubleSignersByHeight(_ context.Context, height uint64) ([]*lib.DoubleSigner, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.doubleSigners == nil {
		return []*lib.DoubleSigner{}, nil
	}
	return data.doubleSigners, nil
}

func (c *BlobClient) CommitteesDataByHeight(_ context.Context, height uint64) ([]*lib.CommitteeData, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.committeesData == nil {
		return []*lib.CommitteeData{}, nil
	}
	return data.committeesData, nil
}

func (c *BlobClient) SubsidizedCommitteesByHeight(_ context.Context, height uint64) ([]uint64, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.subsidizedCommittees == nil {
		return []uint64{}, nil
	}
	return data.subsidizedCommittees, nil
}

func (c *BlobClient) RetiredCommitteesByHeight(_ context.Context, height uint64) ([]uint64, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.retiredCommittees == nil {
		return []uint64{}, nil
	}
	return data.retiredCommittees, nil
}

func (c *BlobClient) SupplyByHeight(_ context.Context, height uint64) (*fsm.Supply, error) {
	data, err := c.dataForHeight(height)
	if err != nil {
		return nil, err
	}
	if data.supply == nil {
		return &fsm.Supply{}, nil
	}
	return data.supply, nil
}

func (c *BlobClient) Poll(_ context.Context) (fsm.Poll, error) {
	return nil, fmt.Errorf("poll not available in blob client")
}

func (c *BlobClient) Proposals(_ context.Context) (fsm.GovProposals, error) {
	return nil, fmt.Errorf("proposals not available in blob client")
}

func (c *BlobClient) dataForHeight(height uint64) (*blobData, error) {
	blob, err := c.blobForHeight(height)
	if err != nil {
		return nil, err
	}
	if blob == c.blobs.GetCurrent() {
		c.currentOnce.Do(func() {
			c.currentData, c.currentErr = decodeBlob(blob)
		})
		if c.currentErr != nil {
			return nil, c.currentErr
		}
		return c.currentData, nil
	}

	c.previousOnce.Do(func() {
		c.previousData, c.previousErr = decodeBlob(blob)
	})
	if c.previousErr != nil {
		return nil, c.previousErr
	}
	return c.previousData, nil
}

func (c *BlobClient) blobForHeight(height uint64) (*fsm.IndexerBlob, error) {
	if c.blobs == nil {
		return nil, fmt.Errorf("blob client has no blobs")
	}
	if height == 0 || height == c.height {
		if c.blobs.Current == nil {
			return nil, fmt.Errorf("current blob missing")
		}
		return c.blobs.Current, nil
	}
	if height+1 == c.height {
		if c.blobs.Previous == nil {
			return nil, fmt.Errorf("previous blob missing for height %d", height)
		}
		return c.blobs.Previous, nil
	}
	return nil, fmt.Errorf("height %d not covered by blob (current %d)", height, c.height)
}

func decodeBlob(blob *fsm.IndexerBlob) (*blobData, error) {
	if blob == nil {
		return nil, fmt.Errorf("blob is nil")
	}

	out := &blobData{
		subsidizedCommittees: blob.SubsidizedCommittees,
		retiredCommittees:    blob.RetiredCommittees,
	}

	if len(blob.Block) > 0 {
		var block lib.BlockResult
		if err := lib.Unmarshal(blob.Block, &block); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}
		out.block = &block
	}

	if len(blob.Params) > 0 {
		var params fsm.Params
		if err := lib.Unmarshal(blob.Params, &params); err != nil {
			return nil, fmt.Errorf("unmarshal params: %w", err)
		}
		out.params = &params
	}

	if len(blob.Supply) > 0 {
		var supply fsm.Supply
		if err := lib.Unmarshal(blob.Supply, &supply); err != nil {
			return nil, fmt.Errorf("unmarshal supply: %w", err)
		}
		out.supply = &supply
	}

	if len(blob.CommitteesData) > 0 {
		var committees lib.CommitteesData
		if err := lib.Unmarshal(blob.CommitteesData, &committees); err != nil {
			return nil, fmt.Errorf("unmarshal committees data: %w", err)
		}
		out.committeesData = committees.List
	}

	if blob.Accounts != nil {
		out.accounts = make([]*fsm.Account, 0, len(blob.Accounts))
		for _, raw := range blob.Accounts {
			var acc fsm.Account
			if err := lib.Unmarshal(raw, &acc); err != nil {
				return nil, fmt.Errorf("unmarshal account: %w", err)
			}
			out.accounts = append(out.accounts, &acc)
		}
	}

	if blob.Pools != nil {
		out.pools = make([]*fsm.Pool, 0, len(blob.Pools))
		for _, raw := range blob.Pools {
			var pool fsm.Pool
			if err := lib.Unmarshal(raw, &pool); err != nil {
				return nil, fmt.Errorf("unmarshal pool: %w", err)
			}
			out.pools = append(out.pools, &pool)
		}
	}

	if blob.Validators != nil {
		out.validators = make([]*fsm.Validator, 0, len(blob.Validators))
		for _, raw := range blob.Validators {
			var validator fsm.Validator
			if err := lib.Unmarshal(raw, &validator); err != nil {
				return nil, fmt.Errorf("unmarshal validator: %w", err)
			}
			out.validators = append(out.validators, &validator)
		}
	}

	if blob.DexPrices != nil {
		out.dexPrices = make([]*lib.DexPrice, 0, len(blob.DexPrices))
		for _, raw := range blob.DexPrices {
			var price lib.DexPrice
			if err := lib.Unmarshal(raw, &price); err != nil {
				return nil, fmt.Errorf("unmarshal dex price: %w", err)
			}
			out.dexPrices = append(out.dexPrices, &price)
		}
	}

	if blob.NonSigners != nil {
		out.nonSigners = make([]*fsm.NonSigner, 0, len(blob.NonSigners))
		for _, raw := range blob.NonSigners {
			var nonSigner fsm.NonSigner
			if err := lib.Unmarshal(raw, &nonSigner); err != nil {
				return nil, fmt.Errorf("unmarshal non-signer: %w", err)
			}
			out.nonSigners = append(out.nonSigners, &nonSigner)
		}
	}

	if blob.DoubleSigners != nil {
		out.doubleSigners = make([]*lib.DoubleSigner, 0, len(blob.DoubleSigners))
		for _, raw := range blob.DoubleSigners {
			var doubleSigner lib.DoubleSigner
			if err := lib.Unmarshal(raw, &doubleSigner); err != nil {
				return nil, fmt.Errorf("unmarshal double-signer: %w", err)
			}
			out.doubleSigners = append(out.doubleSigners, &doubleSigner)
		}
	}

	if len(blob.Orders) > 0 {
		var orderBooks lib.OrderBooks
		if err := lib.Unmarshal(blob.Orders, &orderBooks); err != nil {
			return nil, fmt.Errorf("unmarshal orders: %w", err)
		}
		var orders []*lib.SellOrder
		for _, book := range orderBooks.OrderBooks {
			orders = append(orders, book.Orders...)
		}
		out.orders = orders
	}

	if blob.DexBatches != nil {
		out.dexBatches = make([]*lib.DexBatch, 0, len(blob.DexBatches))
		for _, raw := range blob.DexBatches {
			var batch lib.DexBatch
			if err := lib.Unmarshal(raw, &batch); err != nil {
				return nil, fmt.Errorf("unmarshal dex batch: %w", err)
			}
			out.dexBatches = append(out.dexBatches, &batch)
		}
	}

	if blob.NextDexBatches != nil {
		out.nextDexBatches = make([]*lib.DexBatch, 0, len(blob.NextDexBatches))
		for _, raw := range blob.NextDexBatches {
			var batch lib.DexBatch
			if err := lib.Unmarshal(raw, &batch); err != nil {
				return nil, fmt.Errorf("unmarshal next dex batch: %w", err)
			}
			out.nextDexBatches = append(out.nextDexBatches, &batch)
		}
	}

	return out, nil
}
