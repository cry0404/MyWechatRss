import { useState, useCallback, useRef, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Query } from "@tanstack/react-query";
import { api, type QRStatus } from "@/lib/api";

type QRPhase = "idle" | "loading" | "scanning" | "scanned" | "success" | "expired" | "error";

export function useQRLogin(modalOpen: boolean) {
  const [phase, setPhase] = useState<QRPhase>("idle");
  const [qrImage, setQrImage] = useState<string>("");
  const [qrId, setQrId] = useState<string>("");
  const [deviceName, setDeviceName] = useState<string>("");
  const deviceNameRef = useRef(deviceName);
  deviceNameRef.current = deviceName;
  const openRef = useRef(modalOpen);
  openRef.current = modalOpen;
  const genRef = useRef(0);

  const queryClient = useQueryClient();

  const { mutate: startQR, isPending: isStartingQR } = useMutation({
    mutationFn: api.startQRLogin,
  });

  const fetchNewQR = useCallback(() => {
    genRef.current += 1;
    const gen = genRef.current;
    setQrImage("");
    setQrId("");
    setPhase("loading");
    startQR(deviceNameRef.current, {
      onSuccess: (data) => {
        if (gen !== genRef.current) return;
        if (!openRef.current) return;
        setQrImage(data.qr_image);
        setQrId(data.qr_id);
        setPhase("scanning");
      },
      onError: () => {
        if (gen !== genRef.current) return;
        if (!openRef.current) return;
        setPhase("error");
      },
    });
  }, [startQR]);

  const pollQuery = useQuery({
    queryKey: ["qr-poll", qrId],
    queryFn: () => api.pollQRLogin(qrId),
    enabled: modalOpen && (phase === "scanning" || phase === "scanned") && qrId.length > 0,
    staleTime: 0,
    retry: false,
    refetchIntervalInBackground: true,
    refetchInterval: (query: Query<QRStatus, Error>) => {
      const d = query.state.data;
      if (d?.status === "confirmed" || d?.status === "expired" || d?.status === "cancelled") {
        return false;
      }
      if (query.state.fetchStatus !== "idle") return false;
      return 2000;
    },
  });

  useEffect(() => {
    if (!modalOpen || !openRef.current) return;
    if (pollQuery.isError) {
      setPhase("error");
      return;
    }
    if (!pollQuery.data) return;
    const status = pollQuery.data.status;
    if (status === "scanned") {
      setPhase("scanned");
    } else if (status === "confirmed") {
      setPhase("success");
    } else if (status === "expired" || status === "cancelled") {
      setPhase("expired");
    }
  }, [modalOpen, pollQuery.data]);

  const reset = useCallback(() => {
    genRef.current += 1;
    setPhase("idle");
    setQrImage("");
    setQrId("");
    setDeviceName("");
    queryClient.removeQueries({ queryKey: ["qr-poll"] });
  }, [queryClient]);

  useEffect(() => {
    if (!modalOpen) {
      genRef.current += 1;
      setPhase("idle");
      setQrImage("");
      setQrId("");
      setDeviceName("");
      queryClient.removeQueries({ queryKey: ["qr-poll"] });
    }
  }, [modalOpen, queryClient]);

  return {
    phase,
    qrImage,
    start: fetchNewQR,
    reset,
    isLoading: isStartingQR,
    deviceName,
    setDeviceName,
  };
}
