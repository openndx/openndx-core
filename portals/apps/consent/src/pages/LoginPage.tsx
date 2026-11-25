/**
 * SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Lanka Software Foundation
 */

import { Shield } from 'lucide-react';
import React, { useEffect } from 'react';
import { useAuth } from "react-oidc-context";
import { useNavigate } from "react-router-dom";

const LoginPage: React.FC = () => {

  const navigate = useNavigate();

  const auth = useAuth();
  const isSignedIn = auth.isAuthenticated;
  const isLoading = auth.isLoading;

  useEffect(() => {
    // Check if user is already signed in
    if (!isLoading && isSignedIn) {
      const consentId = localStorage.getItem('consentId');
      if (consentId) {
        console.log('User is signed in, redirecting to consent page');
        navigate('/');
      } else {
        console.log('User is signed in but no consent ID found');
        navigate('/error');
      }
    }
  }, [isSignedIn, isLoading, navigate]);

  const handleSignIn = async () => {
    try {
      console.log('Initiating sign in...');
      await auth.signinRedirect();
    } catch (error) {
      console.error('Sign in error:', error);
    }
  };

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4 relative">
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <Shield className="h-12 w-12 text-blue-500 mx-auto mb-4" />
        <h1 className="text-2xl font-bold text-gray-800 mb-4">Consent Portal</h1>
        <p className="text-gray-600 mb-4">
          You need to sign in to process your consent request.
        </p>
        <button
          onClick={handleSignIn}
          className="bg-blue-500 hover:bg-blue-600 text-white px-6 py-3 rounded-lg font-medium transition-colors"
        >
          Sign In to Continue
        </button>
      </div>
    </div>
  );
};

export default LoginPage;