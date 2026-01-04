package transform

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/canopy-network/canopy/fsm"
	"google.golang.org/protobuf/encoding/protojson"
)

// ProposalFields holds extracted fields from a proposal JSON message.
// Proposals are governance messages (changeParameter, daoTransfer) that require validator approval.
// Uses value types with defaults (0, ”) to match non-Nullable ClickHouse columns.
type ProposalFields struct {
	ProposalType       string // Message type: changeParameter, daoTransfer
	Signer             string // Signer address (hex)
	StartHeight        uint64 // Proposal start height
	EndHeight          uint64 // Proposal end height
	ParameterSpace     string // For changeParameter: fee, val, cons, gov
	ParameterKey       string // For changeParameter: parameter name
	ParameterValue     string // For changeParameter: parameter value as string
	DaoTransferAddress string // For daoTransfer: recipient address
	DaoTransferAmount  uint64 // For daoTransfer: amount
}

// ExtractProposalFields parses a proposal JSON (from fsm.GovProposalWithVote.Proposal)
// and extracts type-specific fields for efficient database querying.
//
// The proposal JSON structure is:
//
//	{
//	  "msg": {
//	    "signer": "...",
//	    "startHeight": 5,
//	    "endHeight": 50,
//	    "parameterSpace": "fee",     // changeParameter only
//	    "parameterKey": "send",      // changeParameter only
//	    "parameterValue": "15",      // changeParameter only
//	    "address": "...",            // daoTransfer only
//	    "amount": 1000               // daoTransfer only
//	  },
//	  "type": "changeParameter"
//	}
func ExtractProposalFields(proposalJSON json.RawMessage) (*ProposalFields, error) {
	// Parse the outer structure to get the message type
	var proposalWrapper struct {
		Msg  json.RawMessage `json:"msg"`
		Type string          `json:"type"`
	}
	if err := json.Unmarshal(proposalJSON, &proposalWrapper); err != nil {
		return nil, fmt.Errorf("unmarshal proposal wrapper: %w", err)
	}

	fields := &ProposalFields{
		ProposalType: proposalWrapper.Type,
	}

	// Extract fields based on message type
	// Uses value assignments (not pointers) to match non-Nullable ClickHouse columns.
	switch proposalWrapper.Type {
	case "changeParameter":
		var msg fsm.MessageChangeParameter
		if err := json.Unmarshal(proposalWrapper.Msg, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageChangeParameter: %w", err)
		}

		fields.Signer = hex.EncodeToString(msg.Signer)
		fields.StartHeight = msg.StartHeight
		fields.EndHeight = msg.EndHeight
		fields.ParameterSpace = msg.ParameterSpace
		fields.ParameterKey = msg.ParameterKey

		// Extract ParameterValue from Any type as JSON string
		if msg.ParameterValue != nil {
			paramValueJSON, err := protojson.Marshal(msg.ParameterValue)
			if err == nil {
				fields.ParameterValue = string(paramValueJSON)
			}
		}

	case "daoTransfer":
		var msg fsm.MessageDAOTransfer
		if err := json.Unmarshal(proposalWrapper.Msg, &msg); err != nil {
			return nil, fmt.Errorf("unmarshal MessageDAOTransfer: %w", err)
		}

		fields.Signer = hex.EncodeToString(msg.Address) // For daoTransfer, the signer is the address field
		fields.StartHeight = msg.StartHeight
		fields.EndHeight = msg.EndHeight
		fields.DaoTransferAddress = hex.EncodeToString(msg.Address)
		fields.DaoTransferAmount = msg.Amount

	default:
		// Unknown proposal type - extract common fields only
		var commonMsg struct {
			Signer      []byte `json:"signer"`
			StartHeight uint64 `json:"startHeight"`
			EndHeight   uint64 `json:"endHeight"`
		}
		if err := json.Unmarshal(proposalWrapper.Msg, &commonMsg); err != nil {
			return nil, fmt.Errorf("unmarshal common proposal fields: %w", err)
		}

		fields.Signer = hex.EncodeToString(commonMsg.Signer)
		fields.StartHeight = commonMsg.StartHeight
		fields.EndHeight = commonMsg.EndHeight
	}

	return fields, nil
}
