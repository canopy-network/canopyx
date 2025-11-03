#!/bin/bash
# Validates database schemas match models and RPC types

set -e

echo "🔍 Validating Database Schemas..."
echo ""

ERRORS=0

# Check validators table schema
echo "Checking validators table schema..."
VALIDATOR_SCHEMA_FILE="pkg/db/chain/validator.go"

if [ -f "$VALIDATOR_SCHEMA_FILE" ]; then
    # Check for required columns in CREATE TABLE statement
    REQUIRED_COLUMNS=(
        "address String"
        "public_key String"
        "net_address String"
        "staked_amount UInt64"
        "committees Array(UInt64)"
        "max_paused_height UInt64"
        "unstaking_height UInt64"
        "output String"
        "delegate UInt8"
        "compound UInt8"
        "status"
        "height UInt64"
        "height_time DateTime64"
    )

    FOUND_COLUMNS=0
    for col in "${REQUIRED_COLUMNS[@]}"; do
        if grep "$col" "$VALIDATOR_SCHEMA_FILE" | grep -qv "//"; then
            FOUND_COLUMNS=$((FOUND_COLUMNS + 1))
        else
            echo "❌ ERROR: Column '$col' not found in validators table schema"
            ERRORS=$((ERRORS + 1))
        fi
    done

    echo "  Found $FOUND_COLUMNS/13 required columns in validators table"

    # Check for phantom commission_rate column
    if grep -v "//" "$VALIDATOR_SCHEMA_FILE" | grep -q "commission_rate"; then
        echo "❌ ERROR: Phantom 'commission_rate' column found in validators table"
        ERRORS=$((ERRORS + 1))
    else
        echo "  ✅ No phantom commission_rate column"
    fi

    # Verify staging table implementation
    if grep -q "ValidatorsStagingTableName" "$VALIDATOR_SCHEMA_FILE"; then
        echo "  ✅ validators_staging table creation found"
    else
        echo "⚠️  WARNING: validators_staging table creation not found"
    fi

else
    echo "❌ ERROR: Validator schema file not found"
    ERRORS=$((ERRORS + 1))
fi

echo ""

# Check validator_signing_info table schema
echo "Checking validator_signing_info table schema..."
SIGNING_INFO_SCHEMA_FILE="pkg/db/chain/validator_signing_info.go"

if [ -f "$SIGNING_INFO_SCHEMA_FILE" ]; then
    REQUIRED_SIGNING_COLUMNS=(
        "address String"
        "missed_blocks_count UInt64"
        "missed_blocks_window UInt64"
        "last_signed_height UInt64"
        "start_height UInt64"
        "height UInt64"
        "height_time DateTime64"
    )

    FOUND_SIGNING=0
    for col in "${REQUIRED_SIGNING_COLUMNS[@]}"; do
        if grep "$col" "$SIGNING_INFO_SCHEMA_FILE" | grep -qv "//"; then
            FOUND_SIGNING=$((FOUND_SIGNING + 1))
        fi
    done

    echo "  Found $FOUND_SIGNING/7 required columns in validator_signing_info table"

    if [ $FOUND_SIGNING -lt 7 ]; then
        echo "❌ ERROR: validator_signing_info table missing columns"
        ERRORS=$((ERRORS + 1))
    fi

    # Verify staging table implementation
    if grep -q "ValidatorSigningInfoStagingTableName" "$SIGNING_INFO_SCHEMA_FILE"; then
        echo "  ✅ validator_signing_info_staging table creation found"
    else
        echo "⚠️  WARNING: validator_signing_info_staging table creation not found"
    fi

else
    echo "❌ ERROR: ValidatorSigningInfo schema file not found"
    ERRORS=$((ERRORS + 1))
fi

echo ""

# Check committee_validators table schema
echo "Checking committee_validators table schema..."
COMMITTEE_VAL_SCHEMA_FILE="pkg/db/chain/committee_validator.go"

if [ -f "$COMMITTEE_VAL_SCHEMA_FILE" ]; then
    # Check for required columns
    if grep "committee_id UInt64" "$COMMITTEE_VAL_SCHEMA_FILE" | grep -qv "//"; then
        echo "  ✅ committee_id column found"
    else
        echo "❌ ERROR: committee_id column not found"
        ERRORS=$((ERRORS + 1))
    fi

    if grep "validator_address String" "$COMMITTEE_VAL_SCHEMA_FILE" | grep -qv "//"; then
        echo "  ✅ validator_address column found"
    else
        echo "❌ ERROR: validator_address column not found"
        ERRORS=$((ERRORS + 1))
    fi

    # Check for phantom commission_rate column
    if grep -v "//" "$COMMITTEE_VAL_SCHEMA_FILE" | grep -q "commission_rate"; then
        echo "❌ ERROR: Phantom 'commission_rate' column found in committee_validators table"
        ERRORS=$((ERRORS + 1))
    else
        echo "  ✅ No phantom commission_rate column"
    fi

    # Verify staging table implementation
    if grep -q "CommitteeValidatorStagingTableName\|CommitteeValidatorsStagingTableName" "$COMMITTEE_VAL_SCHEMA_FILE"; then
        echo "  ✅ committee_validators_staging table creation found"
    else
        echo "⚠️  WARNING: committee_validators_staging table creation not found"
    fi

else
    echo "❌ ERROR: CommitteeValidator schema file not found"
    ERRORS=$((ERRORS + 1))
fi

echo ""

# Verify compression codecs are appropriate
echo "Checking compression codecs..."
if [ -f "$VALIDATOR_SCHEMA_FILE" ]; then
    # Check for Delta codec on monotonic fields
    if grep "height UInt64" "$VALIDATOR_SCHEMA_FILE" | grep -q "Delta"; then
        echo "  ✅ Delta codec used for height (monotonic)"
    else
        echo "⚠️  WARNING: Delta codec not found for height field"
    fi

    # Check for DoubleDelta on timestamps
    if grep "height_time" "$VALIDATOR_SCHEMA_FILE" | grep -q "DoubleDelta"; then
        echo "  ✅ DoubleDelta codec used for height_time (timestamp)"
    else
        echo "⚠️  WARNING: DoubleDelta codec not found for height_time field"
    fi
fi

echo ""

# Report results
if [ $ERRORS -eq 0 ]; then
    echo "✅ All database schema validations passed!"
    exit 0
else
    echo "❌ Found $ERRORS schema validation errors"
    echo ""
    echo "Please fix the schema issues identified above."
    echo "See docs/review/IMPLEMENTATION_SUMMARY.md for details."
    exit 1
fi
