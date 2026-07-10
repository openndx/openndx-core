// services/schemaService.ts
import { getIntrospectionQuery, buildClientSchema, buildSchema, printSchema, parse, print, visit, Kind, graphql } from "graphql";
import type { ObjectTypeDefinitionNode, ObjectTypeExtensionNode } from "graphql";
import type { IntrospectionResult, SchemaRegistration, FieldConfiguration, GraphQLType, SchemaSubmission, ApprovedSchema, ApprovedSchemaApiResponse, PendingSchemaApiResponse } from '../types/graphql';

export class SchemaService {
  private static readonly INTROSPECTION_QUERY = getIntrospectionQuery();

  static getUserDefinedTypes(schema: IntrospectionResult): GraphQLType[] {
    return schema.data.__schema.types.filter(type =>
      !type.name.startsWith('__') && // Remove introspection types
      type.kind === 'OBJECT' &&
      type.fields &&
      type.fields.length > 0
    );
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  static getTypeString(type: GraphQLType | { kind: string; name?: string; ofType?: any }): string {
    if (type.kind === 'NON_NULL') {
      return `${this.getTypeString(type.ofType)}!`;
    }
    if (type.kind === 'LIST') {
      return `[${this.getTypeString(type.ofType)}]`;
    }
    return type.name || type.kind;
  };

  static async fetchSchemaFromEndpoint(endpoint: string): Promise<IntrospectionResult> {
    try {
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          query: this.INTROSPECTION_QUERY,
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const result = await response.json();

      if (result.errors) {
        throw new Error(`GraphQL errors: ${result.errors.map((e: { message: string }) => e.message).join(', ')}`);
      }

      return result;
    } catch (error) {
      throw new Error(`Failed to fetch schema: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async parseIntrospectionJSON(file: File): Promise<IntrospectionResult> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = (event) => {
        try {
          const result = JSON.parse(event.target?.result as string);

          // Validate basic structure
          if (!result.data?.__schema?.types) {
            throw new Error('Invalid introspection result format');
          }

          resolve(result);
        } catch (error) {
          reject(new Error(`Failed to parse JSON: ${error instanceof Error ? error.message : 'Invalid JSON format'}`));
        }
      };
      reader.onerror = () => reject(new Error('Failed to read file'));
      reader.readAsText(file);
    });
  }

  static async parseSDL(sdl: string): Promise<IntrospectionResult> {
    try {
      if (!sdl.trim()) {
        throw new Error('Empty SDL string');
      }

      const schema = buildSchema(sdl);

      const result = await graphql({
        schema,
        source: this.INTROSPECTION_QUERY
      });

      if (result.errors?.length) {
        throw new Error(`SDL introspection errors: ${result.errors.map(e => e.message).join(', ')}`);
      }

      if (!result.data || !(result as unknown as { data: { __schema: { types: unknown[] } } }).data.__schema?.types) {
        throw new Error('Invalid introspection result from SDL');
      }
      return result as unknown as IntrospectionResult;
    } catch (error) {
      throw new Error(`Failed to parse SDL: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async getApprovedSchemas(memberId: string): Promise<ApprovedSchema[]> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/schemas`);
      url.searchParams.append('memberId', memberId);
      const response = await fetch(url.toString(), {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch schemas! status: ${response.status}`);
      }

      const result: ApprovedSchemaApiResponse = await response.json();

      // Handle API response structure {count: number, items: Array | null}
      if (result && typeof result === 'object' && 'items' in result) {
        return Array.isArray(result.items) ? result.items : [];
      }

      // Fallback for direct array response
      console.log('Result is not in expected format, returning empty array.');
      return Array.isArray(result) ? result : [];
    } catch (error) {
      throw new Error(`Failed to get approved schemas: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async getSchemaSubmissions(memberId: string): Promise<SchemaSubmission[]> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/schema-submissions`);
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
        throw new Error(`Failed to fetch schema submissions! status: ${response.status}`);
      }

      const result: PendingSchemaApiResponse = await response.json();

      // Handle API response structure {count: number, items: Array | null}
      if (result && typeof result === 'object' && 'items' in result) {
        return Array.isArray(result.items) ? result.items : [];
      }

      // Fallback for direct array response
      console.log('Result is not in expected format, returning empty array.');
      return Array.isArray(result) ? result : [];
    } catch (error) {
      throw new Error(`Failed to get schema submissions: ${error instanceof Error ? error.message : 'Unknown error'}`);
    }
  }

  static async registerSchema(registration: SchemaRegistration): Promise<void> {
    const baseUrl = window.configs.API_URL || import.meta.env.VITE_BASE_PATH || '';
    try {
      const url = new URL(`${baseUrl}/schema-submissions`);
      const response = await fetch(url.toString(), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(registration),
      });

      if (!response.ok) {
        let errorMessage = `Schema registration failed with status: ${response.status}`;

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

  static async generateSDLWithDirectives(
    result: IntrospectionResult,
    configurations: Record<string, Record<string, FieldConfiguration>>
  ): Promise<string> {

    // 1. Build GraphQL schema
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const gqlSchema = buildClientSchema(result.data as unknown as any);

    // 2. Print schema → SDL
    const baseSDL = printSchema(gqlSchema);

    // 3. Add directive definitions
    const sdlWithDirectives = `
      directive @accessControl(type: String) on FIELD_DEFINITION
			directive @source(value: String) on FIELD_DEFINITION
			directive @isOwner(value: Boolean) on FIELD_DEFINITION
			directive @owner(value: String) on FIELD_DEFINITION
			directive @description(value: String) on FIELD_DEFINITION

			${baseSDL}
		`;

    const ast = parse(sdlWithDirectives);
    // 4. Visit AST, attach directives based on configurations
    const modifiedAST = visit(ast, {
      FieldDefinition(node, _key, _parent, _path, ancestors) {
        const parentNode = ancestors[ancestors.length - 1] as ObjectTypeDefinitionNode | ObjectTypeExtensionNode | undefined;
        if (
          !parentNode ||
          (parentNode.kind !== Kind.OBJECT_TYPE_DEFINITION && parentNode.kind !== Kind.OBJECT_TYPE_EXTENSION)
        ) {
          return;
        }

        const typeName = parentNode.name.value;
        const fieldName = node.name.value;
        const config = configurations[typeName]?.[fieldName];
        if (!config) return;

        const directives = [...(node.directives ?? [])];

        if (config.accessControlType) {
          directives.push({
            kind: Kind.DIRECTIVE,
            name: { kind: Kind.NAME, value: "accessControl" },
            arguments: [
              {
                kind: Kind.ARGUMENT,
                name: { kind: Kind.NAME, value: "type" },
                value: { kind: Kind.STRING, value: config.accessControlType },
              },
            ],
          });
        }

        if (config.source) {
          directives.push({
            kind: Kind.DIRECTIVE,
            name: { kind: Kind.NAME, value: "source" },
            arguments: [
              {
                kind: Kind.ARGUMENT,
                name: { kind: Kind.NAME, value: "value" },
                value: { kind: Kind.STRING, value: config.source },
              },
            ],
          });
        }

        if (config.isOwner !== null) {
          directives.push({
            kind: Kind.DIRECTIVE,
            name: { kind: Kind.NAME, value: "isOwner" },
            arguments: [
              {
                kind: Kind.ARGUMENT,
                name: { kind: Kind.NAME, value: "value" },
                value: { kind: Kind.BOOLEAN, value: config.isOwner },
              },
            ],
          });
        }

        if (config.owner) {
          directives.push({
            kind: Kind.DIRECTIVE,
            name: { kind: Kind.NAME, value: "owner" },
            arguments: [
              {
                kind: Kind.ARGUMENT,
                name: { kind: Kind.NAME, value: "value" },
                value: { kind: Kind.STRING, value: config.owner },
              },
            ],
          });
        }

        if (config.description) {
          directives.push({
            kind: Kind.DIRECTIVE,
            name: { kind: Kind.NAME, value: "description" },
            arguments: [
              {
                kind: Kind.ARGUMENT,
                name: { kind: Kind.NAME, value: "value" },
                value: { kind: Kind.STRING, value: config.description },
              },
            ],
          });
        }

        return { ...node, directives };
      },
    });
    // 5. Print AST back to SDL
    return print(modifiedAST);
  }

}