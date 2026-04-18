import { create } from "zustand";

interface AlertState {
  message: string | null;
  show: (message: string) => void;
  hide: () => void;
}

export const useAlertStore = create<AlertState>((set) => ({
  message: null,
  show: (message) => set({ message }),
  hide: () => set({ message: null }),
}));
