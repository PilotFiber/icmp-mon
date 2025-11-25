import { useState, useEffect, useCallback } from 'react';

export function useApi(fetcher, dependencies = [], options = {}) {
  const { immediate = true, initialData = null, onSuccess, onError } = options;

  const [data, setData] = useState(initialData);
  const [loading, setLoading] = useState(immediate);
  const [error, setError] = useState(null);

  const execute = useCallback(async (...args) => {
    setLoading(true);
    setError(null);

    try {
      const result = await fetcher(...args);
      setData(result);
      onSuccess?.(result);
      return result;
    } catch (err) {
      setError(err);
      onError?.(err);
      throw err;
    } finally {
      setLoading(false);
    }
  }, [fetcher, onSuccess, onError]);

  useEffect(() => {
    if (immediate) {
      execute();
    }
  }, dependencies);

  return { data, loading, error, execute, setData };
}

export function usePolling(fetcher, interval = 5000, dependencies = []) {
  const { data, loading, error, execute } = useApi(fetcher, dependencies);

  useEffect(() => {
    const id = setInterval(execute, interval);
    return () => clearInterval(id);
  }, [execute, interval]);

  return { data, loading, error, refresh: execute };
}
