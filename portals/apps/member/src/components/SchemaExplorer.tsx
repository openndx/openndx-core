// components/SchemaExplorer.tsx
import React, { useState } from 'react';
import type { FieldConfiguration as FieldConfig, GraphQLType } from '../types/graphql';
import { requiresOwnerConsent } from '../types/graphql';
import { FieldConfiguration } from './FieldConfiguration';

interface SchemaExplorerProps {
  configurations: Record<string, Record<string, FieldConfig>>;
  userDefinedTypes: GraphQLType[];
  onConfigurationChange: (typeName: string, fieldName: string, config: FieldConfig) => void;
  onSubmit: () => void;
  loading: boolean;
}

export const SchemaExplorer: React.FC<SchemaExplorerProps> = ({
  configurations,
  userDefinedTypes,
  onConfigurationChange,
  onSubmit,
  loading
}) => {
  const [expandedTypes, setExpandedTypes] = useState<Set<string>>(new Set(userDefinedTypes.map(type => type.name)));

  const toggleTypeExpansion = (typeName: string) => {
    const newExpanded = new Set(expandedTypes);
    if (newExpanded.has(typeName)) {
      newExpanded.delete(typeName);
    } else {
      newExpanded.add(typeName);
    }
    setExpandedTypes(newExpanded);
  };

  const isFormValid = () => {
    for (const type of userDefinedTypes) {
      if (!type.fields) continue;
      
      for (const field of type.fields) {
        const config = configurations[type.name]?.[field.name];
        if (!config) {
          return false;
        }
        if (config.isQueryType || config.isUserDefinedTypeField) {
          if (!config.description) {
            return false;
          }
        } else {
          if (!config.policyClass || !config.source || (requiresOwnerConsent(config.policyClass) && !config.owner)) {
            console.error(`Invalid configuration for ${type.name}.${field.name}, field config:`, config);
            return false;
          }
        }
      }
    }
    return true;
  };

  const getFieldCount = () => {
    return userDefinedTypes.reduce((total, type) => 
      total + (type.fields?.length || 0), 0
    );
  };

  const getConfiguredCount = () => {
    let configured = 0;
    userDefinedTypes.forEach(type => {
      if (!type.fields) return;
      type.fields.forEach(field => {
        const config = configurations[type.name]?.[field.name];
        if (config?.isQueryType || config?.isUserDefinedTypeField) {
          if (config?.description) {
            configured++;
          }
        } else {
          if (config?.policyClass && config?.source && (requiresOwnerConsent(config.policyClass) ? config.owner.trim() : true)) {
            configured++;
          }
        }
      });
    });
    return configured;
  };

  const totalFields = getFieldCount();
  const configuredFields = getConfiguredCount();

  return (
    <div className="bg-white p-6 rounded-xl shadow-lg border border-gray-200">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between mb-6 gap-4">
        <h2 className="text-2xl font-bold text-gray-900">Configure Schema Fields</h2>
        <div className="flex items-center space-x-4">
          <div className="text-sm text-gray-600 bg-gray-50 px-3 py-2 rounded-lg">
            <span className="font-medium">Progress:</span> {configuredFields}/{totalFields} fields
          </div>
          <div className="w-24 bg-gray-200 rounded-full h-2">
            <div 
              className="bg-blue-600 h-2 rounded-full transition-all duration-300"
              style={{ width: `${totalFields > 0 ? (configuredFields / totalFields) * 100 : 0}%` }}
            ></div>
          </div>
        </div>
      </div>

      <div className="mb-6 p-4 bg-blue-50 rounded-lg border border-blue-200">
        <div className="flex items-start space-x-3">
          <div className="flex-shrink-0">
            <svg className="w-5 h-5 text-blue-600 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <div className="flex-1">
            <h3 className="text-sm font-semibold text-blue-800 mb-1">Configuration Instructions</h3>
            <p className="text-sm text-blue-700">
              Configure each field by selecting its source type, ownership status, and providing a description. 
              All fields marked with <span className="text-red-600 font-semibold">*</span> are required.
            </p>
          </div>
        </div>
      </div>

      <div className="space-y-6 max-h-96 overflow-y-auto border border-gray-200 rounded-lg p-4 bg-gray-50">
        {userDefinedTypes.map((type) => (
          <div key={type.name} className="bg-white rounded-lg border border-gray-200 shadow-sm">
            <button
              type="button"
              onClick={() => toggleTypeExpansion(type.name)}
              className="w-full flex items-center justify-between p-4 text-left hover:bg-gray-50 rounded-t-lg transition-colors"
            >
              <div className="flex items-center space-x-3">
                <div className="w-8 h-8 bg-blue-100 rounded-full flex items-center justify-center">
                  <span className="text-blue-700 font-bold text-sm">
                    {type.name.charAt(0).toUpperCase()}
                  </span>
                </div>
                <div>
                  <span className="font-semibold text-lg text-gray-900">{type.name}</span>
                  {type.fields && (
                    <span className="text-sm font-normal text-gray-500 ml-2 bg-gray-100 px-2 py-1 rounded">
                      {type.fields.length} fields
                    </span>
                  )}
                </div>
              </div>
              <div className="flex items-center space-x-2">
                <span className="text-xs text-gray-500">
                  {expandedTypes.has(type.name) ? 'Collapse' : 'Expand'}
                </span>
                <div className={`w-5 h-5 text-gray-400 transition-transform duration-200 ${
                  expandedTypes.has(type.name) ? 'rotate-180' : ''
                }`}>
                  <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </div>
              </div>
            </button>

            {type.description && expandedTypes.has(type.name) && (
              <div className="px-4 pb-2">
                <p className="text-sm text-gray-700 italic bg-gray-50 p-3 rounded border-l-4 border-blue-200">
                  {type.description}
                </p>
              </div>
            )}

            {expandedTypes.has(type.name) && type.fields && (
              <div className="px-4 pb-4 space-y-4 border-t border-gray-100">
                <div className="pt-4">
                  {type.fields.map((field) => {
                    const config = configurations[type.name]?.[field.name];
                    return (
                      <FieldConfiguration
                        key={field.name}
                        typeName={type.name}
                        field={field}
                        configuration={config}
                        onChange={onConfigurationChange}
                      />
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      <div className="mt-8 flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-gray-50 p-4 rounded-lg border">
        <div className="flex items-center space-x-3">
          {configuredFields < totalFields && (
            <div className="flex items-center space-x-2 text-amber-700 bg-amber-50 px-3 py-2 rounded-lg border border-amber-200">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.732-.833-2.5 0L4.314 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
              <span className="text-sm font-medium">
                {totalFields - configuredFields} fields remaining
              </span>
            </div>
          )}
          {configuredFields === totalFields && totalFields > 0 && (
            <div className="flex items-center space-x-2 text-green-700 bg-green-50 px-3 py-2 rounded-lg border border-green-200">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span className="text-sm font-medium">
                All fields configured!
              </span>
            </div>
          )}
        </div>

        <button
          type="button"
          onClick={onSubmit}
          disabled={!isFormValid() || loading}
          className="bg-green-600 text-white py-3 px-8 rounded-lg hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-green-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 font-medium shadow-sm disabled:shadow-none"
        >
          {loading ? (
            <div className="flex items-center space-x-2">
              <svg className="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              <span>Submitting...</span>
            </div>
          ) : (
            'Submit Registration'
          )}
        </button>
      </div>
    </div>
  );
};