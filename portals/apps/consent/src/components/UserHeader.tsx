import React from 'react';
import { useAuth } from "react-oidc-context";

interface UserHeaderProps {
  userName: string | null;
  onSignIn: () => void;
  onSignOut: () => void;
}

const UserHeader: React.FC<UserHeaderProps> = ({ userName, onSignIn, onSignOut }) => {
  const auth = useAuth();
  const isSignedIn = auth.isAuthenticated;

  // On a shared device (e.g. an office kiosk) don't leave the previous user's pending consent behind
  const handleSignOut = () => {
    localStorage.removeItem('consentId');
    onSignOut();
  };

  if (!isSignedIn) {
    return (
      <div className="absolute top-4 right-4 flex items-center space-x-4 bg-white rounded-lg shadow-md px-4 py-2">
        <div className="text-sm text-gray-600">
          Please sign in to continue.
        </div>
        <button
          onClick={onSignIn}
          className="bg-blue-500 hover:bg-blue-600 text-white px-4 py-2 rounded-lg"
        >
          Sign In
        </button>
      </div>
    );
  }

  return (
    <div className="absolute top-4 right-4 flex items-center space-x-4 bg-white rounded-lg shadow-md px-4 py-2">
      <div className="text-sm text-gray-600">
        {userName && <>Welcome, <span className="font-medium text-gray-800">{userName}</span></>}
      </div>
      <button
        onClick={handleSignOut}
        className="text-red-600 hover:text-red-800 text-sm font-medium transition-colors"
      >
        Sign Out
      </button>
    </div>
  );
};

export default UserHeader;