import { AlertCircle, CheckCircle, Shield, X } from 'lucide-react';
import React from 'react';
import { useAuth } from '../contexts/AuthContext';
import UserHeader from '../components/UserHeader';
import { useConsent } from '../contexts/ConsentContext';

const StatusInfoPage: React.FC = () => {
  const { consentRecord } = useConsent();
  const { isAuthenticated, userName, signIn, signOut } = useAuth();

  if (!consentRecord) return null;

  const formatFieldName = (field: string): string => {
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

  const formatDate = (dateString: string): string => {
    return new Date(dateString).toLocaleString();
  };

  const getStatusIcon = () => {
    switch (consentRecord.status) {
      case 'approved':
        return <CheckCircle className="h-12 w-12 text-green-500 mx-auto mb-4" />;
      case 'rejected':
        return <X className="h-12 w-12 text-red-500 mx-auto mb-4" />;
      case 'expired':
        return <AlertCircle className="h-12 w-12 text-orange-500 mx-auto mb-4" />;
      case 'revoked':
        return <X className="h-12 w-12 text-gray-500 mx-auto mb-4" />;
      default:
        return <AlertCircle className="h-12 w-12 text-gray-500 mx-auto mb-4" />;
    }
  };

  const getStatusMessage = () => {
    switch (consentRecord.status) {
      case 'approved':
        return {
          title: 'Consent Already Approved',
          message: `This consent request has already been approved on ${formatDate(consentRecord.updatedAt)}.`,
          bgColor: 'from-green-50 to-emerald-100'
        };
      case 'rejected':
        return {
          title: 'Consent Already Rejected',
          message: `This consent request was rejected on ${formatDate(consentRecord.updatedAt)}.`,
          bgColor: 'from-red-50 to-pink-100'
        };
      case 'expired':
        return {
          title: 'Consent Expired',
          message: `This consent request updated on ${formatDate(consentRecord.updatedAt)} and is no longer valid.`,
          bgColor: 'from-orange-50 to-yellow-100'
        };
      case 'revoked':
        return {
          title: 'Consent Revoked',
          message: `This consent was revoked on ${formatDate(consentRecord.updatedAt)} and is no longer active.`,
          bgColor: 'from-gray-50 to-slate-100'
        };
      default:
        return {
          title: 'Consent Status',
          message: `This consent is currently ${consentRecord.status}.`,
          bgColor: 'from-gray-50 to-slate-100'
        };
    }
  };

  const statusInfo = getStatusMessage();

  const handleReturn = () => {
    if (consentRecord.redirectUrl) {
      window.location.href = consentRecord.redirectUrl;
    } else {
      window.close();
    }
  };

  return (
    <div className={`min-h-screen bg-gradient-to-br ${statusInfo.bgColor} flex items-center justify-center p-4 relative`}>
      {isAuthenticated && <UserHeader userName={userName} onSignIn={() => signIn()} onSignOut={() => signOut()} />}
      <div className="max-w-2xl w-full bg-white rounded-lg shadow-lg overflow-hidden">
        <div className="bg-indigo-600 text-white p-6">
          <div className="flex items-center">
            <Shield className="h-8 w-8 mr-3" />
            <div>
              <h1 className="text-2xl font-bold">Consent Status</h1>
              <p className="text-indigo-100">Information about your consent request</p>
            </div>
          </div>
        </div>

        <div className="p-6 text-center">
          {getStatusIcon()}
          <h2 className="text-xl font-bold text-gray-800 mb-2">{statusInfo.title}</h2>
          <p className="text-gray-600 mb-6">{statusInfo.message}</p>

          <div className="mb-6 p-4 bg-gray-50 rounded-lg text-left">
            <h3 className="text-lg font-semibold text-gray-800 mb-3">Consent Details</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
              <div>
                <span className="font-medium text-gray-600">Application:</span>
                <span className="ml-2 text-gray-800">{consentRecord.appName}</span>
              </div>
              <div>
                <span className="font-medium text-gray-600">Owner Email:</span>
                <span className="ml-2 text-gray-800">{consentRecord.ownerEmail}</span>
              </div>
              <div>
                <span className="font-medium text-gray-600">Status:</span>
                <span className={`ml-2 px-2 py-1 rounded-full text-xs font-medium ${consentRecord.status === 'approved' ? 'bg-green-100 text-green-800' :
                  consentRecord.status === 'rejected' ? 'bg-red-100 text-red-800' :
                    consentRecord.status === 'expired' ? 'bg-orange-100 text-orange-800' :
                      'bg-gray-100 text-gray-800'
                  }`}>
                  {consentRecord.status.charAt(0).toUpperCase() + consentRecord.status.slice(1)}
                </span>
              </div>
              <div>
                <span className="font-medium text-gray-600">Created:</span>
                <span className="ml-2 text-gray-800">{formatDate(consentRecord.createdAt)}</span>
              </div>
            </div>
          </div>

          <div className="mb-6 text-left">
            <h3 className="text-lg font-semibold text-gray-800 mb-3">Data Fields</h3>
            <div className="space-y-2">
              {consentRecord.fields.map((field, index) => (
                <div key={index} className="flex items-center p-2 bg-white border border-gray-200 rounded">
                  <div className="h-2 w-2 bg-indigo-400 rounded-full mr-3"></div>
                  <span className="text-gray-700">{field.displayName || formatFieldName(field.fieldName)}</span>
                </div>
              ))}
            </div>
          </div>

          <button
            onClick={handleReturn}
            className="bg-indigo-600 text-white px-6 py-2 rounded-lg hover:bg-indigo-700 transition-colors"
          >
            Return to Application
          </button>
        </div>
      </div>
    </div>
  );
};

export default StatusInfoPage;