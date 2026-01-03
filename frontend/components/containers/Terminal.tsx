"use client";

import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

interface TerminalProps {
  containerId: string;
  agentId: string;
  onClose?: () => void;
}

export function Terminal({ containerId, agentId, onClose }: TerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!terminalRef.current) return;

    // Initialize xterm.js
    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: "#1a1a2e",
        foreground: "#e4e4e7",
        cursor: "#e4e4e7",
        cursorAccent: "#1a1a2e",
        selectionBackground: "#3b3b5c",
        black: "#1a1a2e",
        red: "#f87171",
        green: "#4ade80",
        yellow: "#fbbf24",
        blue: "#60a5fa",
        magenta: "#c084fc",
        cyan: "#22d3ee",
        white: "#e4e4e7",
        brightBlack: "#52525b",
        brightRed: "#fca5a5",
        brightGreen: "#86efac",
        brightYellow: "#fde047",
        brightBlue: "#93c5fd",
        brightMagenta: "#d8b4fe",
        brightCyan: "#67e8f9",
        brightWhite: "#fafafa",
      },
      rows: 24,
      cols: 80,
    });

    term.open(terminalRef.current);
    xtermRef.current = term;

    // Write welcome message
    term.writeln("\x1b[1;34m╔════════════════════════════════════════╗\x1b[0m");
    term.writeln("\x1b[1;34m║\x1b[0m       InfraPilot Container Shell       \x1b[1;34m║\x1b[0m");
    term.writeln("\x1b[1;34m╚════════════════════════════════════════╝\x1b[0m");
    term.writeln("");
    term.writeln(`\x1b[33mContainer:\x1b[0m ${containerId.slice(0, 12)}`);
    term.writeln("");

    // Connect to WebSocket
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    // Use current window location for WebSocket
    const wsHost = process.env.NEXT_PUBLIC_WS_URL || `${wsProtocol}//${window.location.host}`;
    // Get token from localStorage for WebSocket auth
    const token = localStorage.getItem("access_token");
    const wsUrl = `${wsHost}/api/v1/agents/${agentId}/containers/${containerId}/exec${token ? `?token=${encodeURIComponent(token)}` : ""}`;

    term.writeln(`\x1b[90mConnecting...\x1b[0m`);

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        setConnected(true);
        setError(null);
        term.writeln("\x1b[32mConnected!\x1b[0m");
        term.writeln("");
        term.write("$ ");
      };

      ws.onmessage = (event) => {
        term.write(event.data);
      };

      ws.onerror = () => {
        setError("WebSocket connection failed");
        term.writeln("\x1b[31mConnection error.\x1b[0m");
        term.writeln("\x1b[90mCheck that the container is running and accessible.\x1b[0m");
        term.writeln("");
      };

      ws.onclose = () => {
        setConnected(false);
        term.writeln("");
        term.writeln("\x1b[33mConnection closed.\x1b[0m");
      };

      // Handle user input
      term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(data);
        }
      });
    } catch (err) {
      setError("Failed to create WebSocket connection");
      term.writeln("\x1b[31mFailed to connect.\x1b[0m");
    }

    // Cleanup
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
      }
    };
  }, [containerId, agentId]);

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700">
        <div className="flex items-center gap-3">
          <div className="flex gap-1.5">
            <div className="w-3 h-3 rounded-full bg-red-500" />
            <div className="w-3 h-3 rounded-full bg-yellow-500" />
            <div className="w-3 h-3 rounded-full bg-green-500" />
          </div>
          <span className="text-sm text-gray-400 font-mono">
            {containerId.slice(0, 12)} - /bin/sh
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`text-xs px-2 py-0.5 rounded ${
              connected
                ? "bg-green-500/20 text-green-400"
                : "bg-yellow-500/20 text-yellow-400"
            }`}
          >
            {connected ? "Connected" : "Disconnected"}
          </span>
          {onClose && (
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-white text-sm"
            >
              Close
            </button>
          )}
        </div>
      </div>
      <div
        ref={terminalRef}
        className="flex-1 p-2 bg-[#1a1a2e]"
        style={{ minHeight: "400px" }}
      />
      {error && (
        <div className="px-4 py-2 bg-yellow-500/10 border-t border-yellow-500/30 text-yellow-400 text-xs">
          {error}
        </div>
      )}
    </div>
  );
}
