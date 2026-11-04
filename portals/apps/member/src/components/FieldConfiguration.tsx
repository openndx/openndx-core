// components/FieldConfiguration.tsx
import React from 'react';
import type { GraphQLField } from '../types/graphql';
import type { FieldConfiguration as FieldConfig, PolicyClass } from '../types/graphql';
import { SchemaService } from '../services/schemaService';

const POLICY_CLASS_OPTIONS: { value: PolicyClass; label: string; hint: string }[] = [
  {
    value: 'allowlist',
    label: 'Approved applications',
    hint: 'Applications on the allow-list read it without asking the owner.',
  },
  {
    value: 'allowlist_consent',
    label: 'Approved applications, with owner consent',
    hint: 'Applications on the allow-list also need the owner to consent.',
  },
];

interface FieldConfigurationProps {
  typeName: string;
  field: GraphQLField;
  configuration: FieldConfig;
  onChange: (typeName: string, fieldName: string, config: FieldConfig) => void;
}

export const FieldConfiguration: React.FC<FieldConfigurationProps> = ({
  typeName,
  field,
  configuration,
  onChange
}) => {

  const handlePolicyClassChange = (policyClass: PolicyClass) => {
    // Only consent fields need an owner: it's whose consent is asked
    const owner = policyClass === 'allowlist_consent' ? configuration.owner || 'citizen' : '';
    onChange(typeName, field.name, { ...configuration, policyClass, owner });
  };

  const handleSourceChange = (source: 'primary' | 'fallback') => {
    onChange(typeName, field.name, { ...configuration, source });
  };

  const handleOwnerChange = (owner: 'citizen' | '') => {
    onChange(typeName, field.name, { ...configuration, owner });
  };

  const handleDescriptionChange = (description: string) => {
    onChange(typeName, field.name, { ...configuration, description });
  };

  return (
    <div className="border-l-4 border-blue-300 bg-slate-50 rounded-r-lg pl-6 pr-4 py-4 mb-6 shadow-sm hover:shadow-md transition-shadow duration-200">
      <div className="flex flex-col lg:flex-row lg:items-start gap-6">
        <div className="flex-1 min-w-0">
          <div className="font-medium text-gray-900 mb-2">
            <span className="text-blue-700 font-semibold text-lg">{field.name}</span>
            <span className="text-gray-600 ml-3 text-base font-mono bg-gray-100 px-2 py-1 rounded">
              {SchemaService.getTypeString(field.type)}
            </span>
          </div>
          
          {field.description && (
            <p className="text-sm text-gray-700 mb-3 leading-relaxed bg-white p-3 rounded border-l-2 border-blue-200">
              {field.description}
            </p>
          )}

          {field.args && field.args.length > 0 && (
            <div className="text-xs text-gray-600 mb-3 bg-white p-2 rounded border">
              <span className="font-medium">Arguments:</span> {field.args.map(arg => `${arg.name}: ${SchemaService.getTypeString(arg.type)}`).join(', ')}
            </div>
          )}
        </div>

        <div className="flex-1 space-y-4 bg-white p-4 rounded-lg border border-gray-200">
          {/* Access Policy Configuration */}
          {(!configuration.isQueryType && !configuration.isUserDefinedTypeField) && (
            <div className="bg-gray-50 p-3 rounded-lg border">
              <label className="block text-sm font-semibold text-gray-800 mb-3">
                Access Policy <span className="text-red-600">*</span>
              </label>
              <div className="flex flex-col space-y-2">
                {POLICY_CLASS_OPTIONS.map((option) => (
                  <label key={option.value} className="flex items-start cursor-pointer hover:bg-white p-2 rounded transition-colors">
                    <input
                      type="radio"
                      name={`policyClass-${typeName}-${field.name}`}
                      value={option.value}
                      checked={configuration.policyClass === option.value}
                      onChange={() => handlePolicyClassChange(option.value)}
                      className="mr-2 mt-0.5 h-4 w-4 text-blue-600 focus:ring-blue-500 focus:ring-2"
                      required
                    />
                    <span>
                      <span className="block text-sm font-medium text-gray-700">{option.label}</span>
                      <span className="block text-xs text-gray-500">{option.hint}</span>
                    </span>
                  </label>
                ))}
              </div>
            </div>
          )}
          {/* Source Configuration */}
          {(!configuration.isQueryType && !configuration.isUserDefinedTypeField) && (
            <div className="bg-gray-50 p-3 rounded-lg border">
              <label className="block text-sm font-semibold text-gray-800 mb-3">
                Source <span className="text-red-600">*</span>
              </label>
              <div className="flex flex-col sm:flex-row sm:space-x-4 space-y-2 sm:space-y-0">
                {(['primary', 'fallback'] as const).map((option) => (
                  <label key={option} className="flex items-center cursor-pointer hover:bg-white p-2 rounded transition-colors">
                    <input
                      type="radio"
                      name={`source-${typeName}-${field.name}`}
                      value={option}
                      checked={configuration.source === option}
                      onChange={() => handleSourceChange(option)}
                      className="mr-2 h-4 w-4 text-blue-600 focus:ring-blue-500 focus:ring-2"
                      required
                    />
                    <span className="text-sm font-medium capitalize text-gray-700">{option}</span>
                  </label>
                ))}
              </div>
            </div>
          )}

          {/* Owner Identifier */}
          {(!configuration.isQueryType && !configuration.isUserDefinedTypeField && configuration.policyClass === 'allowlist_consent') && (
            <div className="bg-gray-50 p-3 rounded-lg border">
                <label className="block text-sm font-semibold text-gray-800 mb-2">
                Owner <span className="text-red-600">*</span>
                </label>
              <select
              name={`owner-${typeName}-${field.name}`}
              value={configuration.owner}
              onChange={(e) => handleOwnerChange(e.target.value as 'citizen' | '')}
              className="w-full text-sm px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors"
              required
              >
              <option value="citizen">Citizen</option>
              </select>
            </div>
          )}

          {/* Description */}
          <div className="bg-gray-50 p-3 rounded-lg border">
            <label 
              htmlFor={`desc-${typeName}-${field.name}`}
              className="block text-sm font-semibold text-gray-800 mb-2"
            >
              {(configuration.isUserDefinedTypeField && !configuration.isQueryType) && (
                <div className="bg-blue-50 border border-blue-200 text-blue-800 text-xs p-2 rounded mb-2">
                  <strong>Note:</strong> This is a user-defined type field. You need to specify the source and ownership for all fields inside this user-defined type.
                </div>
              )}
              Description
              {(configuration.isQueryType || configuration.isUserDefinedTypeField) && <span className="text-red-600 ml-1">*</span>}
            </label>
            <textarea
              id={`desc-${typeName}-${field.name}`}
              value={configuration.description}
              onChange={(e) => handleDescriptionChange(e.target.value)}
              placeholder="Describe this field's purpose and data source..."
              rows={3}
              className="w-full text-sm px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors resize-vertical"
              required={configuration.isQueryType || configuration.isUserDefinedTypeField}
            />
          </div>
        </div>
      </div>
    </div>
  );
};