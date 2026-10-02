import { LogIn, Shield } from 'lucide-react';
import React from 'react';
import { useAuth } from 'react-oidc-context';

// Sign-in card for the self-service pages (/consents and /consents/:consentId)
const SignInPrompt: React.FC = () => {
  const auth = useAuth();

  const handleSignIn = () => {
    // The sign-in callback lands on "/", which opens a single consent when a consentId is stored.
    // Drop any left behind (e.g. by a previous user on a shared kiosk) so the owner reaches their list.
    localStorage.removeItem('consentId');
    auth.signinRedirect();
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4">
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg overflow-hidden">
        <div className="bg-indigo-600 text-white p-6 text-center">
          <Shield className="h-12 w-12 mx-auto mb-2" />
          <h1 className="text-2xl font-bold">Consent Portal</h1>
        </div>
        <div className="p-8 text-center">
          <p className="text-gray-600 mb-6">
            Sign in with your own account to review requests to access your data.
          </p>
          <button
            onClick={handleSignIn}
            className="flex w-full items-center justify-center px-4 py-3 border border-transparent text-base font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 md:py-4 md:text-lg transition-colors"
          >
            <LogIn className="mr-2 h-5 w-5" />
            Sign In
          </button>
        </div>
      </div>
    </div>
  );
};

export default SignInPrompt;
