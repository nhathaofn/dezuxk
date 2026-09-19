import React, { createContext, useContext, useState, useEffect } from "react";
import type { User, AuthContextType } from "@/types";
import { loginUser, registerUser, logoutUser, getCurrentUser } from "@/lib/api/auth";

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  useEffect(() => {
    async function checkAuth() {
      try {
        const currentUser = await getCurrentUser();
        if (currentUser) {
          setUser(currentUser);
        } else {
          // The backend memory session is the source of truth. Never restore
          // access from stale browser storage.
          setUser(null);
        }
      } catch (err) {
        console.warn("Failed to check current user from backend:", err);
        setUser(null);
      } finally {
        setIsLoading(false);
      }
    }

    checkAuth();
  }, []);

  const login = async (username: string, password: string) => {
    const loggedInUser = await loginUser(username, password);
    setUser(loggedInUser);
  };

  const register = async (username: string, password: string) => {
    const registeredUser = await registerUser(username, password);
    setUser(registeredUser);
  };

  const logout = async () => {
    try {
      await logoutUser();
    } catch (err) {
      console.error("Backend logout error:", err);
    } finally {
      setUser(null);
    }
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: !!user,
        isLoading,
        login,
        register,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
