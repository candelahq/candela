// Firebase client SDK initialization.
// Config values come from environment variables (set in .env.local or App Hosting).
// Only initializes in the browser — SSR/SSG pages skip Firebase.

import { initializeApp, getApps, type FirebaseApp } from "firebase/app";
import { getAuth, GoogleAuthProvider, type Auth } from "firebase/auth";

const firebaseConfig = {
  apiKey: process.env.NEXT_PUBLIC_FIREBASE_API_KEY,
  authDomain: process.env.NEXT_PUBLIC_FIREBASE_AUTH_DOMAIN,
  projectId: process.env.NEXT_PUBLIC_FIREBASE_PROJECT_ID,
  appId: process.env.NEXT_PUBLIC_FIREBASE_APP_ID,
};

function getFirebaseApp(): FirebaseApp | null {
  if (typeof window === "undefined") return null; // SSR/SSG — skip
  if (!firebaseConfig.apiKey) return null; // Not configured
  return getApps().length === 0 ? initializeApp(firebaseConfig) : getApps()[0];
}

function getFirebaseAuth(): Auth | null {
  const app = getFirebaseApp();
  if (!app) return null;
  return getAuth(app);
}

export const firebaseAuth = getFirebaseAuth();
export const googleProvider = new GoogleAuthProvider();
