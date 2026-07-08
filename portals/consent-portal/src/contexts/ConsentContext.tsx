import React, { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { useAuth } from "react-oidc-context";
import { useLocation, useNavigate } from 'react-router-dom';
import { ConsentStatus } from "../constants/consentStatus";
import { PortalAction } from "../constants/portalAction";
import type { ConsentRecord } from "../types";

interface ConsentContextType {
  consentRecord: ConsentRecord | null;
  error: string;
  isSubmitting: boolean;
  consentId: string | null;

  handleConsentDecision: (decision: PortalAction) => Promise<void>;
  isFetchingConsent: boolean;
  signIn: () => void;
}

const ConsentContext = createContext<ConsentContextType | undefined>(undefined);

// eslint-disable-next-line react-refresh/only-export-components
export const useConsent = () => {
  const context = useContext(ConsentContext);
  if (!context) {
    throw new Error("useConsent must be used within a ConsentProvider");
  }
  return context;
};

export const ConsentProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const navigate = useNavigate();
  const location = useLocation();
  const auth = useAuth();

  const [consentRecord, setConsentRecord] = useState<ConsentRecord | null>(null);
  const [error, setError] = useState('');
  const [consentId, setConsentId] = useState<string | null>(() => {
    // Initial state from localStorage or null
    return localStorage.getItem('consentId');
  });

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isFetchingConsent, setIsFetchingConsent] = useState(false);

  const CONSENT_ENGINE_PATH = window?.configs?.consentEngineUrl;

  // 1. Handle Consent ID Extraction
  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    const idFromUrl = searchParams.get('consentId');

    if (idFromUrl) {
      if (idFromUrl !== consentId) {
        console.log('ConsentContext: Found consentId in URL, updating state and storage:', idFromUrl);
        setConsentId(idFromUrl);
        localStorage.setItem('consentId', idFromUrl);
      }
    }
    // If not in URL, we already initialized from localStorage in useState initializer.
  }, [location.search, consentId]);


  // 2. Fetch Consent when Auth is Ready + Consent ID is present
  useEffect(() => {
    // Only attempt to fetch if:
    // a) We are authenticated
    // b) We have a consent ID
    // c) We haven't fetched it yet (record is null) or we want to re-fetch on ID change
    // d) We are not currently fetching

    if (auth.isLoading) return;

    if (!consentId) {
      // No consent ID to fetch
      return;
    }

    if (!auth.isAuthenticated) {
      // Not logged in, so we can't fetch yet. 
      // The UI will likely show a Login button.
      return;
    }

    // Check if we already have the correct record
    if (consentRecord && consentRecord.consentId === consentId) {
      return;
    }

    const fetchConsent = async () => {
      setIsFetchingConsent(true);
      setError('');

      try {
        console.log('ConsentContext: Fetching consent record for:', consentId);
        const token = auth.user?.access_token;

        if (!token) {
          // Should not happen if isAuthenticated is true
          throw new Error("No access token available");
        }

        const response = await fetch(`${CONSENT_ENGINE_PATH}/consents/${consentId}`, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        });

        if (!response.ok) {
          if (response.status === 403) {
            navigate('/unauthorized');
            throw new Error('You are not authorized to view this consent request.');
          }
          throw new Error(`Failed to load consent details (Status: ${response.status})`);
        }

        const data: ConsentRecord = await response.json();
        // Ensure the internal ID matches
        setConsentRecord({ ...data, consentId: consentId });

      } catch (err: unknown) {
        console.error("ConsentContext: Fetch Error", err);
        const errorMessage = err instanceof Error ? err.message : "Failed to load consent request.";
        setError(errorMessage);
        navigate('/error');
      } finally {
        setIsFetchingConsent(false);
      }
    };

    fetchConsent();

  }, [consentId, auth.isAuthenticated, auth.isLoading, auth.user, CONSENT_ENGINE_PATH, consentRecord, navigate]);

  const handleConsentDecision = async (decision: PortalAction) => {
    if (!consentRecord || !consentRecord.consentId) return;

    if (consentRecord.status !== ConsentStatus.pending) {
      console.warn("Consent is already decided:", consentRecord.status);
      return;
    }

    setIsSubmitting(true);
    try {
      const token = auth.user?.access_token;
      if (!token) throw new Error("Not authenticated");

      const payload = {
        action: decision
      };

      const response = await fetch(`${CONSENT_ENGINE_PATH}/consents/${consentRecord.consentId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify(payload)
      });

      if (!response.ok) {
        throw new Error('Failed to submit decision');
      }

      // Success
      localStorage.removeItem('consentId');
      navigate('/success');

      // Delay closing or redirect logic
      setTimeout(() => {
        if (window.opener) {
          window.opener.postMessage("consentGranted", "*");
        }
        if (consentRecord.redirectUrl) {
          window.location.href = consentRecord.redirectUrl;
        } else {
          window.close();
        }
      }, 3000);

    } catch (err: unknown) {
      console.error("ConsentContext: Decision Error", err);
      const errorMessage = err instanceof Error ? err.message : "Failed to submit decision";
      setError(errorMessage);
      navigate('/error');
    } finally {
      setIsSubmitting(false);
    }
  };

  const signIn = () => {
    auth.signinRedirect();
  };

  return (
    <ConsentContext.Provider
      value={{
        consentRecord,
        error,
        isSubmitting,
        consentId,
        handleConsentDecision,
        isFetchingConsent,
        signIn
      }}
    >
      {children}
    </ConsentContext.Provider>
  );
};