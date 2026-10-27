import { useQuery } from '@tanstack/react-query';
import type { WeatherCity } from '@/api';
import { http } from '@/lib/http';

const weatherCityQueryKey = (query: string) => ['dynamic', 'weather-cities', query] as const;

export type WeatherCityResult = WeatherCity;

export function useWeatherCitySearch(query: string, enabled: boolean) {
  const q = query.trim();
  return useQuery({
    queryKey: weatherCityQueryKey(q),
    queryFn: async () => {
      const data = await http.get<WeatherCityResult[]>('/weather/cities', {
        params: { q },
      });
      return data;
    },
    enabled: enabled && q.length > 0,
    staleTime: 60 * 60 * 1000,
  });
}
