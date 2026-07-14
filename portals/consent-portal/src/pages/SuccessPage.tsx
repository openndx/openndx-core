import { CheckCircle } from 'lucide-react';
import React from 'react';
import { useAuth } from '../contexts/AuthContext';
import UserHeader from '../components/UserHeader';

const SuccessPage: React.FC = () => {
  const { isAuthenticated, userName, signIn, signOut } = useAuth();

  return (
    <div className="min-h-screen bg-gradient-to-br from-green-50 to-emerald-100 flex items-center justify-center p-4 relative">
      {isAuthenticated && <UserHeader userName={userName} onSignIn={() => signIn()} onSignOut={() => signOut()} />}
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <CheckCircle className="h-12 w-12 text-green-500 mx-auto mb-4" />
        <h1 className="text-xl font-bold text-gray-800 mb-2">Success!</h1>
        <p className="text-gray-600 mb-4">
          Your consent has been processed successfully.
        </p>
        <p className="text-sm text-gray-500">
          Redirecting you back to the application...
        </p>
      </div>
    </div>
  );
};

export default SuccessPage;