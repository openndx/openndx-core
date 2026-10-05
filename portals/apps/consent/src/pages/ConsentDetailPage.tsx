import { ArrowLeft, Clock, Shield } from 'lucide-react';
import React, { useCallback, useEffect, useState } from 'react';
import { useAuth } from 'react-oidc-context';
import { Link, useNavigate, useParams } from 'react-router-dom';
import PageSpinner from '../components/PageSpinner';
import SignInPrompt from '../components/SignInPrompt';
import StatusBadge from '../components/StatusBadge';
import UserHeader from '../components/UserHeader';
import { ConsentStatus } from '../constants/consentStatus';
import { PortalAction } from '../constants/portalAction';
import type { ConsentRecord } from '../types';
import { formatDate, formatFieldName, formatGrantDuration } from '../utils/format';
import type { ConsentListNotice } from './ConsentListPage';

// Full view of one of the owner's consent requests, reached from the self-service list. The owner
// reviews exactly what is being requested here before approving or denying it.
const ConsentDetailPage: React.FC = () => {
  const { consentId } = useParams<{ consentId: string }>();
  const auth = useAuth();
  const navigate = useNavigate();
  const { isAuthenticated, isLoading: isAuthLoading, user } = auth;
  const userName = user?.profile?.given_name || user?.profile?.name || user?.profile?.email || user?.profile?.preferred_username || user?.profile?.sub || null;
  const token = user?.access_token;
  const consentEngineUrl = window?.configs?.consentEngineUrl;

  const [consent, setConsent] = useState<ConsentRecord | null>(null);
  const [isFetching, setIsFetching] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [decisionError, setDecisionError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const fetchConsent = useCallback(async () => {
    if (!token || !consentId) return;

    setIsFetching(true);
    setLoadError('');
    try {
      const response = await fetch(`${consentEngineUrl}/consents/${consentId}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (response.status === 401) {
        throw new Error('Your session has expired. Please sign in again.');
      }
      if (response.status === 403) {
        throw new Error('This consent request belongs to a different user.');
      }
      if (response.status === 404 || response.status === 400) {
        throw new Error('This consent request could not be found.');
      }
      if (!response.ok) {
        throw new Error(`Failed to load the consent request (Status: ${response.status})`);
      }

      const data: ConsentRecord = await response.json();
      setConsent(data);
    } catch (err: unknown) {
      console.error('ConsentDetailPage: Fetch Error', err);
      setLoadError(err instanceof Error ? err.message : 'Failed to load the consent request.');
    } finally {
      setIsFetching(false);
    }
  }, [token, consentId, consentEngineUrl]);

  useEffect(() => {
    if (isAuthenticated) {
      fetchConsent();
    }
  }, [isAuthenticated, fetchConsent]);

  const handleDecision = async (decision: PortalAction) => {
    if (!token || !consent) return;

    setIsSubmitting(true);
    setDecisionError('');
    const appName = consent.appName || 'the application';
    try {
      const response = await fetch(`${consentEngineUrl}/consents/${consentId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify({ action: decision })
      });

      if (response.status === 409) {
        setDecisionError(`This request is no longer pending. It may have expired or already been decided.`);
        fetchConsent();
        return;
      }
      if (!response.ok) {
        throw new Error(`Failed to submit your decision (Status: ${response.status})`);
      }

      const notice: ConsentListNotice = {
        kind: 'success',
        message: decision === PortalAction.approve
          ? `You approved the request from ${appName}. The requester can now retry their request.`
          : `You denied the request from ${appName}. Your data will not be shared.`,
      };
      navigate('/consents', { state: { notice } });
    } catch (err: unknown) {
      console.error('ConsentDetailPage: Decision Error', err);
      setDecisionError(err instanceof Error ? err.message : 'Failed to submit your decision.');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (isAuthLoading) {
    return <PageSpinner />;
  }

  if (!isAuthenticated) {
    return <SignInPrompt />;
  }

  const isPending = consent?.status === ConsentStatus.pending;
  const grantDuration = formatGrantDuration(consent?.grantDuration);

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 relative">
      <UserHeader userName={userName} onSignIn={() => auth.signinRedirect()} onSignOut={() => auth.signoutRedirect()} />
      <div className="max-w-2xl mx-auto py-8 pt-20">
        <Link to="/consents" className="inline-flex items-center text-sm text-indigo-700 hover:text-indigo-900 mb-4">
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to my consent requests
        </Link>

        <div className="bg-white rounded-lg shadow-lg overflow-hidden">
          <div className="bg-indigo-600 text-white p-6">
            <div className="flex items-center">
              <Shield className="h-8 w-8 mr-3" />
              <div>
                <h1 className="text-2xl font-bold">Consent Request</h1>
                <p className="text-indigo-100">Review what is being requested before you decide</p>
              </div>
            </div>
          </div>

          <div className="p-6">
            {isFetching && !consent && (
              <div className="flex justify-center py-12">
                <div className="animate-spin rounded-full h-10 w-10 border-b-2 border-indigo-600"></div>
              </div>
            )}

            {loadError && (
              <div className="p-4 rounded-lg bg-red-50 text-red-800 text-sm">{loadError}</div>
            )}

            {consent && !loadError && (
              <>
                <div className="mb-6 flex items-start justify-between">
                  <div>
                    <h2 className="text-xl font-semibold text-gray-800">{consent.appName || consent.appId}</h2>
                    <p className="text-gray-600 text-sm">is requesting access to the following data</p>
                  </div>
                  <StatusBadge status={consent.status} />
                </div>

                <div className="mb-6">
                  <h3 className="text-lg font-semibold text-gray-800 mb-3">Data Fields</h3>
                  <div className="space-y-2">
                    {consent.fields.map((field, index) => (
                      <div key={index} className="flex items-center p-3 bg-gray-50 border border-gray-200 rounded">
                        <div className="h-2 w-2 bg-indigo-400 rounded-full mr-3 flex-shrink-0"></div>
                        <span className="text-gray-700 font-medium">{field.displayName || formatFieldName(field.fieldName)}</span>
                        {field.description && <span className="ml-2 text-gray-600">{field.description}</span>}
                      </div>
                    ))}
                  </div>
                </div>

                <div className="mb-6 p-4 bg-gray-50 rounded-lg">
                  <h3 className="text-lg font-semibold text-gray-800 mb-3">Details</h3>
                  <dl className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
                    <div>
                      <dt className="inline font-medium text-gray-600">Requested:</dt>
                      <dd className="inline ml-2 text-gray-800">{formatDate(consent.createdAt)}</dd>
                    </div>
                    <div>
                      <dt className="inline font-medium text-gray-600">Last updated:</dt>
                      <dd className="inline ml-2 text-gray-800">{formatDate(consent.updatedAt)}</dd>
                    </div>
                    {grantDuration && (
                      <div>
                        <dt className="inline font-medium text-gray-600">Access duration:</dt>
                        <dd className="inline ml-2 text-gray-800">{grantDuration}</dd>
                      </div>
                    )}
                    {consent.status === ConsentStatus.approved && consent.grantExpiresAt && (
                      <div>
                        <dt className="inline font-medium text-gray-600">Access until:</dt>
                        <dd className="inline ml-2 text-gray-800">{formatDate(consent.grantExpiresAt)}</dd>
                      </div>
                    )}
                  </dl>
                </div>

                {decisionError && (
                  <div className="mb-4 p-4 rounded-lg bg-red-50 text-red-800 text-sm">{decisionError}</div>
                )}

                {isPending ? (
                  <>
                    {consent.pendingExpiresAt && (
                      <p className="flex items-center justify-center text-sm text-gray-600 mb-3">
                        <Clock className="h-4 w-4 mr-1" />
                        Respond by {formatDate(consent.pendingExpiresAt)}
                      </p>
                    )}
                    <div className="flex space-x-4">
                      <button
                        onClick={() => handleDecision(PortalAction.reject)}
                        disabled={isSubmitting}
                        className="flex-1 px-6 py-3 bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:bg-red-400 transition-colors font-medium"
                      >
                        {isSubmitting ? 'Processing...' : 'Deny'}
                      </button>
                      <button
                        onClick={() => handleDecision(PortalAction.approve)}
                        disabled={isSubmitting}
                        className="flex-1 px-6 py-3 bg-green-600 text-white rounded-lg hover:bg-green-700 disabled:bg-green-400 transition-colors font-medium"
                      >
                        {isSubmitting ? 'Processing...' : 'Approve'}
                      </button>
                    </div>
                  </>
                ) : (
                  <div className="text-center p-4 bg-gray-50 rounded-lg text-gray-600">
                    This consent request is <strong>{consent.status}</strong> and needs no action.
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default ConsentDetailPage;
