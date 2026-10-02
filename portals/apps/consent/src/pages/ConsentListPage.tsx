import { CheckCircle, ChevronRight, Inbox, RefreshCw, Shield, XCircle } from 'lucide-react';
import React, { useCallback, useEffect, useState } from 'react';
import { useAuth } from 'react-oidc-context';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import PageSpinner from '../components/PageSpinner';
import SignInPrompt from '../components/SignInPrompt';
import StatusBadge from '../components/StatusBadge';
import UserHeader from '../components/UserHeader';
import { ConsentStatus } from '../constants/consentStatus';
import type { ConsentSummary, ListConsentsResponse } from '../types';
import { formatDate } from '../utils/format';

const PAGE_SIZE = 20;

type Filter = 'pending' | 'all';

// Outcome of a decision made on the detail page, passed back through router state
export interface ConsentListNotice {
  kind: 'success' | 'error';
  message: string;
}

const deadlineOf = (consent: ConsentSummary): string => {
  if (consent.status === ConsentStatus.pending && consent.pendingExpiresAt) {
    return `Respond by ${formatDate(consent.pendingExpiresAt)}`;
  }
  if (consent.status === ConsentStatus.approved && consent.grantExpiresAt) {
    return `Access until ${formatDate(consent.grantExpiresAt)}`;
  }
  return '—';
};

// Self-service view of every consent request made for the signed-in data owner. Lets the owner
// find and open requests independently of the requester's session (e.g. a citizen reviewing a
// request an officer raised on their behalf at a counter).
const ConsentListPage: React.FC = () => {
  const auth = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { isAuthenticated, isLoading: isAuthLoading, user } = auth;
  const userName = user?.profile?.given_name || user?.profile?.name || user?.profile?.email || user?.profile?.preferred_username || user?.profile?.sub || null;
  const token = user?.access_token;
  const consentEngineUrl = window?.configs?.consentEngineUrl;

  const [filter, setFilter] = useState<Filter>('pending');
  const [consents, setConsents] = useState<ConsentSummary[]>([]);
  const [total, setTotal] = useState(0);
  const [isFetching, setIsFetching] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [notice, setNotice] = useState<ConsentListNotice | null>(
    () => (location.state as { notice?: ConsentListNotice } | null)?.notice ?? null
  );

  // Consume the notice once so it doesn't reappear on refresh or back navigation
  useEffect(() => {
    if (location.state) {
      navigate(location.pathname, { replace: true, state: null });
    }
  }, [location.state, location.pathname, navigate]);

  const fetchConsents = useCallback(async (offset: number) => {
    if (!token) return;

    setIsFetching(true);
    setLoadError('');
    try {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
      if (filter === 'pending') {
        params.set('status', ConsentStatus.pending);
      }

      const response = await fetch(`${consentEngineUrl}/consents?${params.toString()}`, {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (response.status === 401) {
        throw new Error('Your session has expired. Please sign in again.');
      }
      if (!response.ok) {
        throw new Error(`Failed to load consent requests (Status: ${response.status})`);
      }

      const data: ListConsentsResponse = await response.json();
      setConsents(prev => offset === 0 ? data.consents : [...prev, ...data.consents]);
      setTotal(data.total);
    } catch (err: unknown) {
      console.error('ConsentListPage: Fetch Error', err);
      setLoadError(err instanceof Error ? err.message : 'Failed to load consent requests.');
    } finally {
      setIsFetching(false);
    }
  }, [token, filter, consentEngineUrl]);

  useEffect(() => {
    if (isAuthenticated) {
      fetchConsents(0);
    }
  }, [isAuthenticated, fetchConsents]);

  if (isAuthLoading) {
    return <PageSpinner />;
  }

  if (!isAuthenticated) {
    return <SignInPrompt />;
  }

  const tabClasses = (tab: Filter) =>
    `px-4 py-2 text-sm font-medium rounded-md transition-colors ${filter === tab ? 'bg-indigo-600 text-white' : 'text-gray-600 hover:bg-gray-100'}`;

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 p-4 relative">
      <UserHeader userName={userName} onSignIn={() => auth.signinRedirect()} onSignOut={() => auth.signoutRedirect()} />
      <div className="max-w-4xl mx-auto py-8 pt-20">
        <div className="bg-white rounded-lg shadow-lg overflow-hidden">
          <div className="bg-indigo-600 text-white p-6">
            <div className="flex items-center">
              <Shield className="h-8 w-8 mr-3" />
              <div>
                <h1 className="text-2xl font-bold">My Consent Requests</h1>
                <p className="text-indigo-100">Review who is asking to access your data</p>
              </div>
            </div>
          </div>

          <div className="p-6">
            <div className="flex items-center justify-between mb-4">
              <div className="flex space-x-2">
                <button className={tabClasses('pending')} onClick={() => setFilter('pending')}>Pending</button>
                <button className={tabClasses('all')} onClick={() => setFilter('all')}>All</button>
              </div>
              <button
                onClick={() => fetchConsents(0)}
                disabled={isFetching}
                className="flex items-center text-sm text-indigo-600 hover:text-indigo-800 disabled:text-gray-400"
              >
                <RefreshCw className={`h-4 w-4 mr-1 ${isFetching ? 'animate-spin' : ''}`} />
                Refresh
              </button>
            </div>

            {notice && (
              <div className={`mb-4 p-4 rounded-lg flex items-start ${notice.kind === 'success' ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'}`}>
                {notice.kind === 'success'
                  ? <CheckCircle className="h-5 w-5 mr-2 mt-0.5 flex-shrink-0" />
                  : <XCircle className="h-5 w-5 mr-2 mt-0.5 flex-shrink-0" />}
                <p className="text-sm flex-1">{notice.message}</p>
                <button onClick={() => setNotice(null)} className="text-sm ml-2 opacity-70 hover:opacity-100" aria-label="Dismiss">×</button>
              </div>
            )}

            {loadError && (
              <div className="mb-4 p-4 rounded-lg bg-red-50 text-red-800 text-sm">{loadError}</div>
            )}

            {consents.length === 0 ? (!loadError && !isFetching && (
              <div className="text-center py-12 text-gray-500">
                <Inbox className="h-12 w-12 mx-auto mb-3 text-gray-400" />
                <p>{filter === 'pending' ? 'You have no pending consent requests.' : 'You have no consent requests yet.'}</p>
                {filter === 'pending' && (
                  <p className="text-sm mt-1">If you were told a request is waiting, select Refresh in a moment.</p>
                )}
              </div>
            )) : (
              <div className="overflow-x-auto border border-gray-200 rounded-lg">
                <table className="min-w-full divide-y divide-gray-200 text-sm">
                  <thead className="bg-gray-50">
                    <tr>
                      <th scope="col" className="px-4 py-3 text-left font-semibold text-gray-700">Application</th>
                      <th scope="col" className="px-4 py-3 text-left font-semibold text-gray-700">Requested</th>
                      <th scope="col" className="px-4 py-3 text-left font-semibold text-gray-700">Status</th>
                      <th scope="col" className="px-4 py-3 text-left font-semibold text-gray-700">Deadline</th>
                      <th scope="col" className="px-4 py-3"><span className="sr-only">Open</span></th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200 bg-white">
                    {consents.map(consent => (
                      <tr
                        key={consent.consentId}
                        onClick={() => navigate(`/consents/${consent.consentId}`)}
                        className="cursor-pointer hover:bg-indigo-50 transition-colors"
                      >
                        <td className="px-4 py-3 font-medium text-gray-800">
                          <Link
                            to={`/consents/${consent.consentId}`}
                            onClick={e => e.stopPropagation()}
                            className="hover:text-indigo-700"
                          >
                            {consent.appName || consent.appId}
                          </Link>
                        </td>
                        <td className="px-4 py-3 text-gray-600 whitespace-nowrap">{formatDate(consent.createdAt)}</td>
                        <td className="px-4 py-3"><StatusBadge status={consent.status} /></td>
                        <td className="px-4 py-3 text-gray-600 whitespace-nowrap">{deadlineOf(consent)}</td>
                        <td className="px-4 py-3 text-right text-gray-400"><ChevronRight className="h-4 w-4 inline" /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {isFetching && (
              <div className="flex justify-center py-6">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-indigo-600"></div>
              </div>
            )}

            {!isFetching && consents.length < total && (
              <div className="text-center mt-4">
                <button
                  onClick={() => fetchConsents(consents.length)}
                  className="text-sm text-indigo-600 hover:text-indigo-800 font-medium"
                >
                  Load more
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default ConsentListPage;
