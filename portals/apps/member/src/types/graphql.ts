// types/graphql.ts
export interface GraphQLField {
  name: string;
  type: {
    kind: string;
    name?: string;
    ofType?: GraphQLTypeRef;
  };
  description?: string;
  args?: GraphQLInputValue[];
}

export interface GraphQLInputValue {
  name: string;
  type: {
    kind: string;
    name?: string;
    ofType?: GraphQLTypeRef;
  };
  description?: string;
  defaultValue?: unknown;
}

export interface GraphQLType {
  kind: string;
  name: string;
  description?: string;
  fields?: GraphQLField[];
  inputFields?: GraphQLInputValue[];
  interfaces?: GraphQLType[];
  possibleTypes?: GraphQLType[];
  enumValues?: Array<{
    name: string;
    description?: string;
    isDeprecated?: boolean;
    deprecationReason?: string;
  }>;
  ofType?: GraphQLTypeRef;
}

export interface GraphQLTypeRef {
  kind: string;
  name?: string;
  ofType?: GraphQLTypeRef;
}

export interface IntrospectionResult {
  data: {
    __schema: {
      queryType: { name: string };
      mutationType?: { name: string };
      subscriptionType?: { name: string };
      types: GraphQLType[];
    };
  };
}

export type PolicyClass = 'allowlist' | 'allowlist_consent';

export interface FieldConfiguration {
  policyClass: PolicyClass | ''; // '' indicates not set
  source: 'primary' | 'fallback' | ''; // '' indicates not set
  owner: 'citizen' | ''; // Whose consent is asked; set only for allowlist_consent
  description: string;
  isQueryType: boolean; // Is Field Defined Inside a Query Type
  isUserDefinedTypeField: boolean; // Is this field a User Defined Type field
}

export interface SchemaRegistration {
  schemaName: string;
  schemaDescription?: string;
  sdl: string;
  previousSchemaId: string | null;
  schemaEndpoint: string;
  memberId: string;
}

export interface SchemaSubmission extends SchemaRegistration {
  submissionId: string;
  status: 'pending' | 'approved' | 'rejected';
  createdAt: string; // Note: API uses createdAt, not created_at
  updatedAt: string;
}

export interface ApprovedSchema {
  schemaId: string;
  schemaName: string;
  schemaDescription?: string;
  sdl: string;
  schemaEndpoint: string;
  version: "active" | "deprecated";
  memberId: string;
  createdAt: string;
  updatedAt: string;
}

// API Response structure
export interface PendingSchemaApiResponse {
  count: number;
  items: SchemaSubmission[] | null;
}

export interface ApprovedSchemaApiResponse {
  count: number;
  items: ApprovedSchema[] | null;
}