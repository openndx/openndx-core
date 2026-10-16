import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ConsentStatus } from '../constants/consentStatus';
import { PortalAction } from '../constants/portalAction';
import type { ConsentRecord } from '../types';
import ConsentDetailPage from './ConsentDetailPage';

const mockUseAuth = vi.fn();

vi.mock('react-oidc-context', () => ({
  useAuth: () => mockUseAuth(),
}));

const CONSENT_ENGINE_URL = 'http://consent-engine.test/api/v1';
const CONSENT_ID = 'consent-123';
const ACCESS_TOKEN = 'test-access-token';

function baseConsent(overrides: Partial<ConsentRecord> = {}): ConsentRecord {
  return {
    consentId: CONSENT_ID,
    appId: 'app-1',
    appName: 'Demo App',
    ownerId: 'owner-1',
    ownerEmail: 'owner@example.com',
    status: ConsentStatus.pending,
    type: 'data_access',
    grantDuration: '1d',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    fields: [{ fieldName: 'fullName', schemaId: 'schema-1', owner: 'citizen' }],
    ...overrides,
  };
}

function mockAuth() {
  mockUseAuth.mockReturnValue({
    isAuthenticated: true,
    isLoading: false,
    user: {
      access_token: ACCESS_TOKEN,
      profile: { given_name: 'Ada', email: 'ada@example.com' },
    },
    signinRedirect: vi.fn(),
    signoutRedirect: vi.fn(),
  });
}

function renderDetail() {
  return render(
    <MemoryRouter initialEntries={[`/consents/${CONSENT_ID}`]}>
      <Routes>
        <Route path="/consents/:consentId" element={<ConsentDetailPage />} />
        <Route path="/consents" element={<div>Consent list</div>} />
      </Routes>
    </MemoryRouter>,
  );
}

function mockFetchSequence(handlers: Array<(input: RequestInfo | URL, init?: RequestInit) => Response | Promise<Response>>) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const handler = handlers.shift();
    if (!handler) {
      throw new Error(`Unexpected fetch: ${String(input)} ${init?.method ?? 'GET'}`);
    }
    return handler(input, init);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

describe('ConsentDetailPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    window.configs = {
      consentEngineUrl: CONSENT_ENGINE_URL,
      idpBaseUrl: 'http://idp.test',
      idpClientId: 'consent-portal',
      idpScope: 'openid',
      idpSignInRedirectUrl: 'http://localhost/callback',
      idpSignOutRedirectUrl: 'http://localhost/',
    };
    mockAuth();
  });

  it('shows Approve and Deny for a pending consent, and approving sends the action then leaves for the list', async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      () => jsonResponse(baseConsent({ status: ConsentStatus.pending })),
      (input, init) => {
        expect(String(input)).toBe(`${CONSENT_ENGINE_URL}/consents/${CONSENT_ID}`);
        expect(init?.method).toBe('PUT');
        expect(init?.headers).toMatchObject({
          Authorization: `Bearer ${ACCESS_TOKEN}`,
          'Content-Type': 'application/json',
        });
        expect(JSON.parse(String(init?.body))).toEqual({ action: PortalAction.approve });
        return jsonResponse({ status: ConsentStatus.approved });
      },
    ]);

    renderDetail();

    expect(await screen.findByRole('button', { name: 'Approve' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deny' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Approve' }));

    await waitFor(() => {
      expect(screen.getByText('Consent list')).toBeInTheDocument();
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('shows Revoke access for an approved consent; confirmation sends no request until confirmed', async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      () => jsonResponse(baseConsent({ status: ConsentStatus.approved, grantExpiresAt: '2026-02-01T00:00:00Z' })),
      (input, init) => {
        expect(init?.method).toBe('PUT');
        expect(JSON.parse(String(init?.body))).toEqual({ action: PortalAction.revoke });
        expect(String(input)).toBe(`${CONSENT_ENGINE_URL}/consents/${CONSENT_ID}`);
        expect(init?.headers).toMatchObject({ Authorization: `Bearer ${ACCESS_TOKEN}` });
        return jsonResponse({ status: ConsentStatus.revoked });
      },
      () => jsonResponse(baseConsent({ status: ConsentStatus.revoked })),
    ]);

    renderDetail();

    const revokeButton = await screen.findByRole('button', { name: 'Revoke access' });
    expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await user.click(revokeButton);
    expect(screen.getByText(/Revoke access for/i)).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Keep access' }));
    expect(screen.queryByText(/Revoke access for/i)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Revoke access' })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole('button', { name: 'Revoke access' }));
    await user.click(screen.getByRole('button', { name: 'Revoke access' }));

    expect(await screen.findByText(/You revoked access for Demo App/i)).toBeInTheDocument();
    expect(screen.getByText('Revoked')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it('shows a no-longer-active message on 409 revoke and reloads the consent', async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      () => jsonResponse(baseConsent({ status: ConsentStatus.approved })),
      () => jsonResponse({ error: 'CONSENT_NOT_REVOCABLE' }, 409),
      () => jsonResponse(baseConsent({ status: ConsentStatus.expired })),
    ]);

    renderDetail();

    await user.click(await screen.findByRole('button', { name: 'Revoke access' }));
    await user.click(screen.getByRole('button', { name: 'Revoke access' }));

    expect(await screen.findByText(/This consent is no longer active/i)).toBeInTheDocument();
    expect(screen.getByText('Expired')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it.each([
    ConsentStatus.rejected,
    ConsentStatus.expired,
    ConsentStatus.revoked,
  ] as const)('shows no action buttons for a %s consent', async (status) => {
    mockFetchSequence([() => jsonResponse(baseConsent({ status }))]);

    renderDetail();

    expect(await screen.findByText((_, element) =>
      element?.textContent === `This consent request is ${status} and needs no action.`,
    )).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke access' })).not.toBeInTheDocument();
  });
});
