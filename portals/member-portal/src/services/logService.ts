interface LogEntry {
    id: string;
    timestamp: string;
    status: 'failure' | 'success';
    requestedData: string;
    applicationId: string; // ID of the application making the request
    schemaId: string; // ID of the schema being accessed
    consumerId: string; // Member ID of the consumer
    providerId: string; // Member ID of the provider
}

interface LogResponse {
    logs: LogEntry[] | null;
    total: number;
    limit: number;
    offset: number;
}

interface LogQueryParams {
    consumerId?: string;
    providerId?: string;
    status?: 'success' | 'failure';
    startDate?: string;
    endDate?: string;
    limit?: number;
    offset?: number;
}

export class LogService {

    static async fetchLogsWithParams(params?: LogQueryParams): Promise<LogResponse> {
        try {
            const url = new URL(`${window.configs.LOGS_URL}/logs`);
            
            if (params) {
                Object.entries(params).forEach(([key, value]) => {
                    if (value !== undefined && value !== null && value !== '') {
                        url.searchParams.append(key, value.toString());
                    }
                });
            }
            
            const response = await fetch(url.toString());
            
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            
            const data: LogResponse = await response.json();

            return data;

        } catch (error) {
            console.error('Error fetching logs with params:', error);
            throw error;
        }
    }

    
    /**
     * Export logs data (placeholder for future implementation)
     * @param params - Query parameters for filtering the export
     * @param format - Export format (csv, json, etc.)
     * @returns Promise<Blob>
     */
    static async exportLogs(params?: LogQueryParams, format: 'csv' | 'json' = 'csv'): Promise<Blob> {
        try {
            const url = new URL(`${window.configs.LOGS_URL}/logs/export`);
            url.searchParams.append('format', format);
            
            if (params) {
                Object.entries(params).forEach(([key, value]) => {
                    if (value !== undefined && value !== null && value !== '') {
                        url.searchParams.append(key, value.toString());
                    }
                });
            }
            
            const response = await fetch(url.toString());
            
            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }
            
            return await response.blob();
        } catch (error) {
            console.error('Error exporting logs:', error);
            throw error;
        }
    }

    /**
     * Get log statistics
     * @param params - Query parameters for filtering
     * @returns Promise with log statistics
     */
    static async getLogStatistics(params?: LogQueryParams): Promise<{
        total: number;
        success: number;
        failure: number;
        successRate: number;
    }> {
        try {
            const logResponse = await this.fetchLogsWithParams(params);
            const logs = logResponse.logs || [];
            const total = logs.length;
            const success = logs.filter(log => log.status === 'success').length;
            const failure = logs.filter(log => log.status === 'failure').length;
            const successRate = total > 0 ? (success / total) * 100 : 0;

            return {
                total,
                success,
                failure,
                successRate
            };
        } catch (error) {
            console.error('Error getting log statistics:', error);
            throw error;
        }
    }
}

export type { LogEntry, LogResponse, LogQueryParams };