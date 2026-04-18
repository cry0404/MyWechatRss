import { useMutation } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useAuthStore } from "@/stores/authStore";

export function useLogin() {
  const setToken = useAuthStore((s) => s.setToken);
  return useMutation({
    mutationFn: (data: { username: string; password: string }) =>
      api.login(data.username, data.password),
    onSuccess: (data) => {
      setToken(data.token);
    },
  });
}

export function useLogout() {
  const logout = useAuthStore((s) => s.logout);
  return () => {
    logout();
    window.location.href = "/login";
  };
}
