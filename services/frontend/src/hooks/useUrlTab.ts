import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";

// Sincroniza a aba ativa de um <Tabs> com a query string (?tab=...), para
// que recarregar a página volte para a mesma aba em vez de sempre a
// primeira. Usa replace (não empilha entrada no histórico a cada troca
// de aba).
export function useUrlTab(defaultValue: string, paramName = "tab") {
  const [searchParams, setSearchParams] = useSearchParams();
  const value = searchParams.get(paramName) ?? defaultValue;

  const setValue = useCallback(
    (next: string) => {
      setSearchParams(
        (prev) => {
          const params = new URLSearchParams(prev);
          params.set(paramName, next);
          return params;
        },
        { replace: true },
      );
    },
    [paramName, setSearchParams],
  );

  return [value, setValue] as const;
}
