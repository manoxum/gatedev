import { Component, type ErrorInfo, type ReactNode } from "react";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
}

// Unico jeito de capturar erro de render/commit do React (hooks nao
// tem equivalente a getDerivedStateFromError/componentDidCatch) - sem
// isso, um throw nesse ciclo derruba a arvore inteira e deixa a tela
// em branco (foi o caso do react-gauge-component reagindo a um ref
// nulo apos o componente desmontar em pleno polling).
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[frontend] erro capturado pelo ErrorBoundary", error, info.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback ?? null;
    }
    return this.props.children;
  }
}
