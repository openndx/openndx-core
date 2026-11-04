// pages/SchemaRegistrationPage.tsx
import React, { useState, useEffect } from 'react';
import { AlertCircle } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import type { IntrospectionResult, FieldConfiguration, SchemaRegistration, GraphQLType, ApprovedSchema } from '../types/graphql';
import { SchemaInput } from '../components/SchemaInput';
import { SchemaExplorer } from '../components/SchemaExplorer';
import { SchemaService } from '../services/schemaService';
import { RegistrationSuccess } from '../components/RegistrationSuccess';

interface SchemaRegistrationPageProps {
  memberId: string;
}

export const SchemaRegistrationPage: React.FC<SchemaRegistrationPageProps> = ({
  memberId,
}) => {
  const navigate = useNavigate();
  const [step, setStep] = useState<'input' | 'configure'>('input');
  const [schema, setSchema] = useState<IntrospectionResult | null>(null);
  const [endpoint, setEndpoint] = useState<string>('');
  const [configurations, setConfigurations] = useState<Record<string, Record<string, FieldConfiguration>>>({});
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [userDefinedTypes, setUserDefinedTypes] = useState<GraphQLType[]>([]);
  const [previous_schema, setPreviousSchema] = useState<ApprovedSchema | null>(null);
  const [registeredSchemas, setRegisteredSchemas] = useState<ApprovedSchema[]>([]);
  const [schemaName, setSchemaName] = useState<string>('');
  const [schemaDescription, setSchemaDescription] = useState<string>('');
  const [showSuccess, setShowSuccess] = useState(false);

  useEffect(() => {
    // Fetch registered schemas from the API
    const fetchRegisteredSchemas = async () => {
      try {
        const response: ApprovedSchema[] = await SchemaService.getApprovedSchemas(memberId);
        if (response) {
          setRegisteredSchemas(response);
        }
      } catch {
        setError('Failed to fetch registered schemas');
      }
    };

    fetchRegisteredSchemas();
  }, [memberId]);

  const handleSchemaLoaded = (loadedSchema: IntrospectionResult, endpoint: string) => {
    setSchema(loadedSchema);
    setEndpoint(endpoint);
    setStep('configure');
    setError('');
    // Initialize configurations for all fields
    const initialConfigs: Record<string, Record<string, FieldConfiguration>> = {};

    const userDefinedTypes_ = SchemaService.getUserDefinedTypes(loadedSchema);
    setUserDefinedTypes(userDefinedTypes_);
    loadedSchema.data.__schema.types
      .filter(type =>
        !type.name.startsWith('__') &&
        type.kind === 'OBJECT' &&
        type.fields
      )
      .forEach(type => {
        initialConfigs[type.name] = {};
        if (type.name === "Query") {
          type.fields?.forEach(field => {
            initialConfigs[type.name][field.name] = {
              policyClass: '',
              source: '',
              owner: '',
              description: 'Default Description',
              isQueryType: true,
              isUserDefinedTypeField: false
            };
          });
        }
        else {
          type.fields?.forEach(field => {
            const isUserDefinedTypeField_ = userDefinedTypes_.map(t => t.name).includes(SchemaService.getTypeString(field.type));
            initialConfigs[type.name][field.name] = {
              policyClass: isUserDefinedTypeField_ ? '' : 'allowlist',
              source: isUserDefinedTypeField_ ? '' : 'fallback',
              owner: '',
              description: field.description || isUserDefinedTypeField_ ? 'Default Description' : '',
              isQueryType: false,
              isUserDefinedTypeField: isUserDefinedTypeField_
            };
          });
        }
      });

    setConfigurations(initialConfigs);
  };

  const handleConfigurationChange = (
    typeName: string,
    fieldName: string,
    config: FieldConfiguration
  ) => {
    setConfigurations(prev => ({
      ...prev,
      [typeName]: {
        ...prev[typeName],
        [fieldName]: config
      }
    }));
  };

  const handleSubmitRegistration = async () => {
    if (!schema) {
      setError('Schema is required');
      return;
    }

    setLoading(true);
    setError('');

    try {
      // Generate SDL with directives
      const sdl = await SchemaService.generateSDLWithDirectives(schema, configurations);

      const registration: SchemaRegistration = {
        sdl,
        schemaName: schemaName || 'Untitled Schema',
        schemaDescription,
        previousSchemaId: previous_schema ? previous_schema.schemaId : null,
        schemaEndpoint: endpoint,
        memberId: memberId
      };

      await SchemaService.registerSchema(registration);

      // Show success page on successful registration
      setShowSuccess(true);

    } catch (error) {
      console.error('Error registering schema:', error);
      const errorMessage = error instanceof Error ? error.message : 'Registration failed';
      setError(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleBackToInput = () => {
    setStep('input');
    setSchema(null);
    setConfigurations({});
    setError('');
  };

  const handleSuccessRedirect = () => {
    navigate('/schemas');
  };

  // Show success page after successful registration
  if (showSuccess) {
    return (
      <RegistrationSuccess
        type="schema"
        title={schemaName || 'Schema'}
        onRedirect={handleSuccessRedirect}
      />
    );
  }

  const getSchemaStats = () => {
    if (!schema) return null;

    const types = schema.data.__schema.types.filter(type =>
      !type.name.startsWith('__') && type.kind === 'OBJECT'
    );

    const totalFields = types.reduce((sum, type) =>
      sum + (type.fields?.length || 0), 0
    );

    return {
      types: types.length,
      fields: totalFields,
      queryType: schema.data.__schema.queryType?.name,
      mutationType: schema.data.__schema.mutationType?.name,
      subscriptionType: schema.data.__schema.subscriptionType?.name
    };
  };

  const stats = getSchemaStats();

  return (
    <div className="min-h-screen bg-gray-50 py-8 w-full">
      <div className="max-w-6xl mx-auto px-4 sm:px-6 lg:px-8">
        {/* Header */}
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold text-gray-900 mb-2">
            GraphQL Schema Registration
          </h1>
          <p className="text-gray-600">
            Register your GraphQL schema as a data provider
          </p>
        </div>

        {/* Progress Indicator */}
        <div className="mb-8">
          <div className="flex items-center justify-center space-x-4">
            <div className={`flex items-center space-x-2 ${step === 'input' ? 'text-blue-600' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${step === 'input' ? 'bg-blue-600 text-white' : 'bg-gray-200'
                }`}>
                1
              </div>
              <span className="font-medium">Schema Input</span>
            </div>

            <div className="w-16 h-px bg-gray-300"></div>

            <div className={`flex items-center space-x-2 ${step === 'configure' ? 'text-blue-600' : 'text-gray-400'}`}>
              <div className={`w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium ${step === 'configure' ? 'bg-blue-600 text-white' : 'bg-gray-200'
                }`}>
                2
              </div>
              <span className="font-medium">Configure Fields</span>
            </div>
          </div>
        </div>

        {/* Error Alert */}
        {error && (
          <div className="mb-6 p-4 bg-red-50 border border-red-200 rounded-md">
            <div className="flex">
              <div className="flex-shrink-0">
                <AlertCircle className="h-5 w-5 text-red-400" />
              </div>
              <div className="ml-3">
                <h3 className="text-sm font-medium text-red-800">Error</h3>
                <div className="mt-2 text-sm text-red-700">{error}</div>
              </div>
            </div>
          </div>
        )}



        {/* Schema Stats */}
        {stats && step === 'configure' && (
          <div className="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-md">
            <h3 className="text-sm font-medium text-blue-800 mb-2">Schema Information</h3>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
              <div>
                <span className="font-medium text-blue-700">Types:</span>
                <span className="ml-2 text-blue-600">{stats.types}</span>
              </div>
              <div>
                <span className="font-medium text-blue-700">Fields:</span>
                <span className="ml-2 text-blue-600">{stats.fields}</span>
              </div>
              <div>
                <span className="font-medium text-blue-700">Query:</span>
                <span className="ml-2 text-blue-600">{stats.queryType || 'None'}</span>
              </div>
              <div>
                <span className="font-medium text-blue-700">Mutation:</span>
                <span className="ml-2 text-blue-600">{stats.mutationType || 'None'}</span>
              </div>
            </div>
          </div>
        )}

        {/* Step Content */}
        {step === 'input' && (
          <SchemaInput
            onSchemaLoaded={handleSchemaLoaded}
            onError={setError}
            loading={loading}
            setLoading={setLoading}
          />
        )}

        {step === 'configure' && schema && (
          <div className="space-y-6">
            {/* Provider ID Input */}
            <div className="bg-white p-6 rounded-lg shadow-md">
              <div>
                <label htmlFor="schemaName" className="block text-sm font-medium text-gray-700 mb-2">
                  Schema Name <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  id="schemaName"
                  value={schemaName}
                  onChange={(e) => setSchemaName(e.target.value)}
                  placeholder="Enter schema name..."
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors"
                />
              </div>
              <div className="mt-4">
                <label htmlFor="schemaDescription" className="block text-sm font-medium text-gray-700 mb-2">
                  Schema Description
                </label>
                <textarea
                  id="schemaDescription"
                  value={schemaDescription}
                  onChange={(e) => setSchemaDescription(e.target.value)}
                  placeholder="Enter schema description..."
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors"
                  rows={3}
                />
              </div>
              {/* Previous Schema Selection */}
              <div className="mt-4">
                <label htmlFor="previousSchemaId" className="block text-sm font-medium text-gray-700 mb-2">
                  Previous Schema
                </label>
                <select
                  id="previousSchemaId"
                  value={previous_schema?.schemaId || ''}
                  onChange={(e) => {
                    const selectedId = e.target.value;
                    if (selectedId) {
                      const selectedSchema = registeredSchemas.find(schema => schema.schemaId.toString() === selectedId);
                      setPreviousSchema(selectedSchema || null);
                    } else {
                      setPreviousSchema(null);
                    }
                  }}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md focus:ring-blue-500 focus:border-blue-500"
                >
                  <option value="">None</option>
                  {registeredSchemas.map((schema) => (
                    <option key={schema.schemaId} value={schema.schemaId}>
                      {schema.schemaName}
                    </option>
                  ))}
                </select>
              </div>
              <div className="mt-4">
                <label htmlFor="schemaEndpoint" className="block text-sm font-medium text-gray-700 mb-2">
                  Schema Endpoint
                </label>
                <input
                  type="text"
                  id="schemaEndpoint"
                  value={endpoint ? endpoint : ''}
                  placeholder="Schema Endpoint"
                  disabled
                  readOnly
                  className="w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-100 text-gray-600 cursor-not-allowed"
                />
              </div>
            </div>

            {/* Back Button */}
            <div className="flex justify-start">
              <button
                type="button"
                onClick={handleBackToInput}
                className="text-blue-600 hover:text-blue-800 text-sm font-medium flex items-center"
              >
                <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                </svg>
                Back to Schema Input
              </button>
            </div>

            {/* Schema Explorer */}
            <SchemaExplorer
              configurations={configurations}
              userDefinedTypes={userDefinedTypes}
              onConfigurationChange={handleConfigurationChange}
              onSubmit={handleSubmitRegistration}
              loading={loading}
            />
          </div>
        )}
      </div>
    </div>
  );
};