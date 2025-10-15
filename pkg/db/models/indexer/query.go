package indexer

// This file previously contained BlockRow and TransactionRow types which were
// intermediate representations used by query methods. These have been removed
// as part of a refactoring to eliminate code duplication - the database models
// (Block, Transaction, etc.) now have JSON tags and are used directly in API responses.
