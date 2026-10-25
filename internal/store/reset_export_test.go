package store

// Test-only views of the reset tables (reset.go), for the schema check of
// reset_integration_test.go (package store_test).
var (
	ResetOperationTables     = operationTables
	ResetOperationMarkers    = operationMarkers
	ResetConfigurationTables = configurationTables
)
