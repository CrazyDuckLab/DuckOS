/**
 * DuckOS V1 — error boundary.
 *
 * A single root error boundary catches render-time errors that the
 * typed API/state layers cannot. The fallback keeps the shell
 * usable: the operator can still navigate, dismiss the banner, or
 * sign out.
 */
import { Component, type ErrorInfo, type ReactNode } from "react";

export interface ErrorBoundaryProps {
  children: ReactNode;
  onError?: (error: Error, info: ErrorInfo) => void;
}

export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  { error: Error | null }
> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    this.props.onError?.(error, info);
    if (typeof console !== "undefined") {
      console.error("DuckOS V1 shell error:", error, info);
    }
  }

  reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      return (
        <div
          className="v1-state v1-error-screen"
          data-state="error"
          role="alert"
        >
          <div className="state-head">
            <span className="v1-dot" data-state="bad" />
            <h3>The dashboard hit an unexpected error</h3>
          </div>
          <div className="state-body">
            The shell is still functional — you can reload, sign out, or
            navigate to a known-good page. The error has been reported.
          </div>
          <div className="state-actions">
            <button type="button" className="v1-btn" onClick={this.reset}>
              Try again
            </button>
            <button
              type="button"
              className="v1-btn"
              data-variant="ghost"
              onClick={() => {
                if (typeof window !== "undefined") window.location.reload();
              }}
            >
              Reload
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
