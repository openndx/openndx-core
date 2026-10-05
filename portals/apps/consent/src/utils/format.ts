// Turns a dotted/camelCase field path such as "person.dateOfBirth" into "Date Of Birth"
export const formatFieldName = (field: string): string => {
  const lastField = field ? field.split('.').at(-1) : '';
  if (!lastField) return field;

  const words = lastField
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .split(/[_\s]+/)
    .filter(word => word.length > 0);

  return words
    .map(word => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
};

export const formatDate = (dateString: string): string => {
  return new Date(dateString).toLocaleString();
};

const grantDurationLabels: Record<string, string> = {
  PT1H: '1 hour',
  PT6H: '6 hours',
  PT12H: '12 hours',
  P1D: '1 day',
  P7D: '7 days',
  P30D: '30 days',
};

// Human-readable label for an ISO 8601 grant duration returned by the Consent Engine
export const formatGrantDuration = (grantDuration?: string): string | null => {
  if (!grantDuration) return null;
  return grantDurationLabels[grantDuration] ?? grantDuration;
};
