import { GoogleLogin } from "@react-oauth/google";
import { Shield } from "lucide-react";
import React, { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";

const LoginPage: React.FC = () => {
  const navigate = useNavigate();
  const {
    isAuthenticated,
    isLoading,
    error: authError,
    handleGoogleCredential,
  } = useAuth();
  const [googleError, setGoogleError] = useState<string | null>(null);

  const error = authError || googleError;

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      const consentId = localStorage.getItem("consentId");
      if (consentId) {
        console.log("User is signed in, redirecting to consent page");
        navigate("/");
      } else {
        console.log("User is signed in but no consent ID found");
        navigate("/error");
      }
    }
  }, [isAuthenticated, isLoading, navigate]);

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-indigo-600 mx-auto mb-4"></div>
          <p className="text-gray-600">Signing in...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-indigo-100 flex items-center justify-center p-4 relative">
      <div className="max-w-md w-full bg-white rounded-lg shadow-lg p-6 text-center">
        <Shield className="h-12 w-12 text-blue-500 mx-auto mb-4" />
        <h1 className="text-2xl font-bold text-gray-800 mb-4">
          Consent Portal
        </h1>
        <p className="text-gray-600 mb-6">
          You need to sign in to process your consent request.
        </p>

        {error && (
          <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-lg text-red-700 text-sm">
            {error}
          </div>
        )}

        <div className="flex justify-center">
          <GoogleLogin
            onSuccess={(credentialResponse) => {
              setGoogleError(null);
              if (credentialResponse.credential) {
                handleGoogleCredential(credentialResponse.credential);
              } else {
                setGoogleError("No credential received from Google. Please try again.");
              }
            }}
            onError={() => {
              console.error("Google Login Failed");
              setGoogleError(
                "Google Sign-In failed. Please check that pop-ups and third-party cookies are enabled, then try again.",
              );
            }}
            text="signin_with"
            shape="rectangular"
            size="large"
            width="300"
          />
        </div>
      </div>
    </div>
  );
};

export default LoginPage;
