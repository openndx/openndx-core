import React from 'react';
import { ConsentStatus } from '../constants/consentStatus';

const statusBadgeClasses: Record<ConsentStatus, string> = {
  pending: 'bg-yellow-100 text-yellow-800',
  approved: 'bg-green-100 text-green-800',
  rejected: 'bg-red-100 text-red-800',
  expired: 'bg-orange-100 text-orange-800',
  revoked: 'bg-gray-100 text-gray-800',
};

const StatusBadge: React.FC<{ status: ConsentStatus }> = ({ status }) => (
  <span className={`px-2 py-1 rounded-full text-xs font-medium whitespace-nowrap ${statusBadgeClasses[status] ?? 'bg-gray-100 text-gray-800'}`}>
    {status.charAt(0).toUpperCase() + status.slice(1)}
  </span>
);

export default StatusBadge;
