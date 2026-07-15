package federator

import (
	"github.com/OpenNDX/openndx-core/exchange/orchestration-engine/middleware"
)

// convertToAuditFieldRecords converts ProviderLevelFieldRecord to audit package format
func convertToAuditFieldRecords(records *[]ProviderLevelFieldRecord) *[]middleware.ProviderLevelFieldRecord {
	if records == nil {
		return nil
	}

	result := make([]middleware.ProviderLevelFieldRecord, len(*records))
	for i, record := range *records {
		result[i] = middleware.ProviderLevelFieldRecord{
			SchemaId:   record.SchemaId,
			ServiceKey: record.ServiceKey,
			FieldPath:  record.FieldPath,
		}
	}
	return &result
}
