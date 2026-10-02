import { ConsentStatus } from "../constants/consentStatus";

interface ConsentField {
  fieldName: string;
  schemaId: string;
  displayName?: string;
  description?: string;
  owner: string;
}

export interface ConsentRecord {
  consentId?: string;
  appId: string;
  appName?: string;
  ownerId: string;
  ownerEmail: string;
  status: ConsentStatus;
  type: string;
  grantDuration?: string;
  createdAt: string;
  updatedAt: string;
  pendingExpiresAt?: string;
  grantExpiresAt?: string;
  fields: ConsentField[];
  redirectUrl?: string;
}

// Compact row returned when listing the owner's consents; fetch by ID for the full ConsentRecord
export interface ConsentSummary {
  consentId: string;
  appId: string;
  appName?: string;
  status: ConsentStatus;
  createdAt: string;
  pendingExpiresAt?: string;
  grantExpiresAt?: string;
}

export interface ListConsentsResponse {
  consents: ConsentSummary[];
  total: number;
  limit: number;
  offset: number;
}
