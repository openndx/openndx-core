import { AlertCircle } from 'lucide-react';
import React, { useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';
import UserHeader from '../components/UserHeader';
import { useConsent } from '../contexts/ConsentContext';

const ErrorPage: React.FC = () => {
  const { error } = useConsent();
  const { isAuthenticated, isLoading, userName, signIn, signOut } = useAuth();

  useEffect(() => {
    console.log('Auth State:', { isLoading, isAuthenticated });
  }, [isLoading, isAuthenticated]);

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-red-50 to-pink-100 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-red-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-red-50 to-pink-100 flex items-center justify-center p-4 relative">
      <UserHeader
        userName={userName}
        onSignIn={() => signIn()}
        onSignOut={() => signOut()}
      />
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <AlertCircle className="h-12 w-12 text-red-500 mx-auto mb-4" />
        <h1 className="text-xl font-bold text-gray-800 mb-2">Error</h1>
        <p className="text-gray-600 mb-4">
          {error || 'An unexpected error occurred. Please try again.'}
        </p>
        <button
          onClick={() => window.location.href = '/'}
          className="bg-red-600 text-white px-6 py-2 rounded-lg hover:bg-red-700 transition-colors"
        >
          Try Again
        </button>
      </div>
    </div>
  );
};

export default ErrorPage;