import React, {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from "react";

interface AuthState {
  isAuthenticated: boolean;
  isLoading: boolean;
  accessToken: string | null;
  userEmail: string | null;
  userName: string | null;
  error: string | null;
}

interface AuthContextType extends AuthState {
  signIn: () => void;
  signOut: () => void;
  handleGoogleCredential: (idToken: string) => Promise<void>;
}

const AUTH_SESSION_KEY = "consent_portal_auth";

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// eslint-disable-next-line react-refresh/only-export-components
export const useAuth = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
};

/**
 * Decode a JWT payload without verification (for display only).
 * ThunderID performs the actual cryptographic verification server-side.
 *
 * Handles base64url→base64 conversion, missing padding, and multi-byte
 * UTF-8 characters (e.g. accented names) via TextDecoder.
 */
function decodeJwtPayload(token: string): Record<string, unknown> {
  try {
    const parts = token.split(".");
    if (parts.length < 2) return {};
    const payload = parts[1];
    const base64 = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = base64.padEnd(
      base64.length + ((4 - (base64.length % 4)) % 4),
      "=",
    );
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return JSON.parse(new TextDecoder().decode(bytes));
  } catch (error) {
    console.error("[Auth] Failed to decode JWT payload:", error);
    return {};
  }
}

/**
 * Load persisted auth state from sessionStorage.
 * Returns default unauthenticated state if nothing is stored or parsing fails.
 */
function loadPersistedAuth(): AuthState {
  const defaultState: AuthState = {
    isAuthenticated: false,
    isLoading: false,
    accessToken: null,
    userEmail: null,
    userName: null,
    error: null,
  };

  try {
    const stored = sessionStorage.getItem(AUTH_SESSION_KEY);
    if (!stored) return defaultState;
    const parsed = JSON.parse(stored) as Partial<AuthState>;
    // Only restore if we have a valid token
    if (parsed.accessToken && parsed.isAuthenticated) {
      return {
        isAuthenticated: true,
        isLoading: false,
        accessToken: parsed.accessToken,
        userEmail: parsed.userEmail ?? null,
        userName: parsed.userName ?? null,
        error: null,
      };
    }
  } catch {
    sessionStorage.removeItem(AUTH_SESSION_KEY);
  }

  return defaultState;
}

export const AuthProvider: React.FC<{ children: ReactNode }> = ({
  children,
}) => {
  const [state, setState] = useState<AuthState>(loadPersistedAuth);

  const thunderIdTokenUrl = window.configs?.thunderIdTokenUrl || "";
  const clientId = window.configs?.idpClientId || "";

  /**
   * Handle Google credential response (id_token JWT).
   * Called by the GoogleLogin button's onSuccess callback.
   *
   * Flow (mirrors the working Passport App):
   * 1. Receive Google ID token (signed JWT) from Google Identity Services
   * 2. Exchange it with ThunderID via RFC 8693 token exchange
   * 3. ThunderID validates signature, issuer, audience, then returns access_token
   */
  const handleGoogleCredential = useCallback(
    async (googleIdToken: string) => {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        // Decode Google ID token for display (email, name)
        const claims = decodeJwtPayload(googleIdToken);
        const email = (claims.email as string) || null;
        const name =
          (claims.name as string) || (claims.given_name as string) || email;

        console.log("[Auth] Google credential received for:", email);

        // RFC 8693 Token Exchange with ThunderID
        const params = new URLSearchParams({
          grant_type: "urn:ietf:params:oauth:grant-type:token-exchange",
          subject_token: googleIdToken,
          subject_token_type: "urn:ietf:params:oauth:token-type:id_token",
          client_id: clientId,
        });

        const response = await fetch(thunderIdTokenUrl, {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: params.toString(),
        });

        if (!response.ok) {
          const errorBody = await response.text();
          console.error(
            "[Auth] Token exchange failed:",
            response.status,
            errorBody,
          );
          throw new Error(`Token exchange failed (${response.status})`);
        }

        const data = await response.json();
        const accessToken = data.access_token;

        if (!accessToken) {
          throw new Error("ThunderID did not return an access_token");
        }

        console.log("[Auth] Token exchange successful for:", email);

        const newState: AuthState = {
          isAuthenticated: true,
          isLoading: false,
          accessToken,
          userEmail: email,
          userName: name,
          error: null,
        };

        // Persist to sessionStorage so page refreshes don't log the user out
        sessionStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(newState));
        setState(newState);
      } catch (err) {
        console.error("[Auth] Authentication error:", err);
        sessionStorage.removeItem(AUTH_SESSION_KEY);
        setState({
          isAuthenticated: false,
          isLoading: false,
          accessToken: null,
          userEmail: null,
          userName: null,
          error: err instanceof Error ? err.message : "Authentication failed",
        });
      }
    },
    [thunderIdTokenUrl, clientId],
  );

  const signIn = useCallback(() => {
    // Sign-in is triggered by the GoogleLogin button in LoginPage.
    // This is a no-op placeholder; the actual flow starts from the button click.
    console.log("[Auth] signIn called — use the Google Sign-In button");
  }, []);

  const signOut = useCallback(() => {
    // Clear auth state but preserve consentId in localStorage so the user
    // can sign in with a different account and still access the same consent.
    // consentId is removed only on successful consent completion (in ConsentContext).
    sessionStorage.removeItem(AUTH_SESSION_KEY);
    setState({
      isAuthenticated: false,
      isLoading: false,
      accessToken: null,
      userEmail: null,
      userName: null,
      error: null,
    });
  }, []);

  return (
    <AuthContext.Provider
      value={{
        ...state,
        signIn,
        signOut,
        handleGoogleCredential,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};
