#!/bin/bash
# Validates RPC types against Canopy source

set -e

CANOPY_PATH="${CANOPY_PATH:-../canopy}"
CANOPYX_PATH="."

echo "🔍 Validating RPC Types against Canopy source..."
echo "Canopy path: $CANOPY_PATH"
echo "CanopyX path: $CANOPYX_PATH"
echo ""

# Check Canopy path exists
if [ ! -d "$CANOPY_PATH" ]; then
    echo "❌ ERROR: Canopy source not found at $CANOPY_PATH"
    echo "Set CANOPY_PATH environment variable to point to Canopy source"
    exit 1
fi

ERRORS=0

# Validate Validator struct
echo "Checking RpcValidator struct..."

# Check each required field individually in CanopyX
REQUIRED_FIELDS=(
    "Address:string:address"
    "PublicKey:string:publicKey"
    "NetAddress:string:netAddress"
    "StakedAmount:uint64:stakedAmount"
    "Committees:\[\]uint64:committees"
    "MaxPausedHeight:uint64:maxPausedHeight"
    "UnstakingHeight:uint64:unstakingHeight"
    "Output:string:output"
    "Delegate:bool:delegate"
    "Compound:bool:compound"
)

FOUND_COUNT=0
for field_spec in "${REQUIRED_FIELDS[@]}"; do
    IFS=':' read -r field_name field_type json_tag <<< "$field_spec"

    # Check if field exists in CanopyX with correct type and JSON tag
    if grep -A 30 "type RpcValidator struct" "$CANOPYX_PATH/pkg/rpc/types.go" | grep -q "$field_name.*$field_type.*json:\"$json_tag"; then
        FOUND_COUNT=$((FOUND_COUNT + 1))
    else
        echo "❌ ERROR: Field '$field_name $field_type' with json:\"$json_tag\" not found in RpcValidator"
        ERRORS=$((ERRORS + 1))
    fi
done

echo "  Found $FOUND_COUNT/10 required validator fields"

# Check for phantom fields that should NOT exist
PHANTOM_FIELDS="CommissionRate Status"
for field in $PHANTOM_FIELDS; do
    if grep -A 30 "type RpcValidator struct" "$CANOPYX_PATH/pkg/rpc/types.go" | grep -q "^\s*$field\s"; then
        echo "❌ ERROR: Phantom field '$field' found in RpcValidator (not in Canopy)"
        ERRORS=$((ERRORS + 1))
    fi
done

echo ""

# Validate NonSigner struct
echo "Checking RpcNonSigner struct..."

# Check for Counter field with correct JSON tag
if grep -A 10 "type RpcNonSigner struct" "$CANOPYX_PATH/pkg/rpc/types.go" | grep -q 'Counter.*uint64.*json:"counter"'; then
    echo "  ✅ Counter field with correct JSON tag found"
else
    echo "❌ ERROR: Counter field with json:\"counter\" not found in RpcNonSigner"
    ERRORS=$((ERRORS + 1))
fi

# Check for phantom MissedBlocks field
if grep -A 10 "type RpcNonSigner struct" "$CANOPYX_PATH/pkg/rpc/types.go" | grep -q "MissedBlocks"; then
    echo "❌ ERROR: Phantom 'MissedBlocks' field found in RpcNonSigner (should be 'Counter')"
    ERRORS=$((ERRORS + 1))
fi

# Check for phantom WindowStart field
if grep -A 10 "type RpcNonSigner struct" "$CANOPYX_PATH/pkg/rpc/types.go" | grep -q "WindowStart"; then
    echo "❌ ERROR: Phantom 'WindowStart' field found in RpcNonSigner (not in Canopy)"
    ERRORS=$((ERRORS + 1))
fi

echo ""

# Validate database models match RPC types
echo "Checking database models..."

# Check Validator model has all fields
VALIDATOR_MODEL_FILE="$CANOPYX_PATH/pkg/db/models/indexer/validator.go"
if [ -f "$VALIDATOR_MODEL_FILE" ]; then
    MODEL_FIELDS_FOUND=0
    for field_spec in "${REQUIRED_FIELDS[@]}"; do
        IFS=':' read -r field_name field_type json_tag <<< "$field_spec"
        if grep "type Validator struct" -A 50 "$VALIDATOR_MODEL_FILE" | grep -q "$field_name"; then
            MODEL_FIELDS_FOUND=$((MODEL_FIELDS_FOUND + 1))
        fi
    done
    echo "  Found $MODEL_FIELDS_FOUND/10 validator fields in database model"

    if [ $MODEL_FIELDS_FOUND -lt 10 ]; then
        echo "❌ ERROR: Database Validator model missing fields"
        ERRORS=$((ERRORS + 1))
    fi

    # Check for DeriveStatus method
    if grep -q "func.*DeriveStatus" "$VALIDATOR_MODEL_FILE"; then
        echo "  ✅ DeriveStatus() method found"
    else
        echo "❌ ERROR: DeriveStatus() method not found in Validator model"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "⚠️  WARNING: Validator model file not found"
fi

echo ""

# Check RPC interface has height parameters
echo "Checking RPC client interface..."
INTERFACE_FILE="$CANOPYX_PATH/pkg/rpc/interfaces.go"
if [ -f "$INTERFACE_FILE" ]; then
    if grep "Validators.*context.Context.*height uint64" "$INTERFACE_FILE" | grep -q "RpcValidator"; then
        echo "  ✅ Validators(ctx, height) interface found"
    else
        echo "❌ ERROR: Validators() interface missing height parameter"
        ERRORS=$((ERRORS + 1))
    fi

    if grep "NonSigners.*context.Context.*height uint64" "$INTERFACE_FILE" | grep -q "RpcNonSigner"; then
        echo "  ✅ NonSigners(ctx, height) interface found"
    else
        echo "❌ ERROR: NonSigners() interface missing height parameter"
        ERRORS=$((ERRORS + 1))
    fi
else
    echo "⚠️  WARNING: RPC interface file not found"
fi

echo ""

# Report results
if [ $ERRORS -eq 0 ]; then
    echo "✅ All RPC type validations passed!"
    exit 0
else
    echo "❌ Found $ERRORS validation errors"
    echo ""
    echo "Please fix the issues identified above."
    echo "See docs/review/IMPLEMENTATION_SUMMARY.md for detailed fix instructions."
    exit 1
fi
