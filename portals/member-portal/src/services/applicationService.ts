// services/applicationService.ts
import type {
  ApplicationRegistration,
  ApplicationSubmission,
  ApprovedApplication,
  PendingApplicationApiResponse,
  ApprovedApplicationApiResponse
} from '../types/applications';

export class ApplicationService {

  static async getApprovedApplications(memberId: string): Promise<ApprovedApplication[]> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/applications`);
      url.searchParams.append('memberId', memberId);
      const response = await fetch(url.toString(), {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch approved applications! status: ${response.status}`);
      }

      const result: ApprovedApplicationApiResponse = await response.json();

      // Handle API response structure {count: number, items: Array | null}
      if (result && typeof result === 'object' && 'items' in result) {
        return Array.isArray(result.items) ? result.items : [];
      }

      // Fallback for direct array response
      return Array.isArray(result) ? result : [];
    } catch (error) {
      throw new Error(`Failed to get approved applications: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async getApplicationSubmissions(memberId: string): Promise<ApplicationSubmission[]> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/application-submissions`);
      url.searchParams.append('memberId', memberId);
      url.searchParams.append('status', 'pending');
      url.searchParams.append('status', 'rejected');
      const response = await fetch(url.toString(), {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch application submissions! status: ${response.status}`);
      }

      const result: PendingApplicationApiResponse = await response.json();

      // Handle API response structure {count: number, items: Array | null}
      if (result && typeof result === 'object' && 'items' in result) {
        return Array.isArray(result.items) ? result.items : [];
      }

      // Fallback for direct array response
      return Array.isArray(result) ? result : [];
    } catch (error) {
      throw new Error(`Failed to get application submissions: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async registerApplication(registration: ApplicationRegistration): Promise<void> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/application-submissions`);
      const response = await fetch(url.toString(), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(registration),
      });

      if (!response.ok) {
        let errorMessage = `Application registration failed with status: ${response.status}`;

        try {
          // Try to get error details from response
          const errorData = await response.json();
          if (errorData.message) {
            errorMessage += ` - ${errorData.message}`;
          } else if (errorData.error) {
            errorMessage += ` - ${errorData.error}`;
          } else if (typeof errorData === 'string') {
            errorMessage += ` - ${errorData}`;
          }
        } catch {
          // If we can't parse the error response, use the status text
          errorMessage += ` - ${response.statusText || 'Unknown error'}`;
        }

        throw new Error(errorMessage);
      }
    } catch (error) {
      // Re-throw network errors or already formatted errors
      if (error instanceof TypeError && error.message.includes('fetch')) {
        throw new Error('Network error: Unable to connect to the server. Please check your connection and try again.');
      }
      throw error;
    }
  }

  // static async updateApplication(consumerId: string, applicationId: string, updates: Partial<ApplicationRegistration>): Promise<void> {
  //   const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
  //   try {
  //     const response = await fetch(`${baseUrl}/consumers/${consumerId}/applications/${applicationId}`, {
  //       method: 'PUT',
  //       headers: {
  //         'Content-Type': 'application/json',
  //       },
  //       body: JSON.stringify(updates),
  //     });

  //     if (!response.ok) {
  //       throw new Error(`Application update failed! status: ${response.status}`);
  //     }
  //   } catch (error) {
  //     throw new Error(`Failed to update application: ${error instanceof Error ? error.message : 'Unknown error'}`);
  //   }
  // }

  // static async deleteApplication(consumerId: string, applicationId: string): Promise<void> {
  //   const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
  //   try {
  //     const response = await fetch(`${baseUrl}/consumers/${consumerId}/applications/${applicationId}`, {
  //       method: 'DELETE',
  //       headers: {
  //         'Content-Type': 'application/json',
  //       },
  //     });

  //     if (!response.ok) {
  //       throw new Error(`Application deletion failed! status: ${response.status}`);
  //     }
  //   } catch (error) {
  //     throw new Error(`Failed to delete application: ${error instanceof Error ? error.message : 'Unknown error'}`);
  //   }
  // }
}