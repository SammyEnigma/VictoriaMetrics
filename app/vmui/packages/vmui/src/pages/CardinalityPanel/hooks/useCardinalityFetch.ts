import { ErrorTypes } from "../../../types";
import { useAppState } from "../../../state/common/StateContext";
import { useEffect, useRef, useState } from "preact/compat";
import { CardinalityRequestsParams, getCardinalityInfo, getMetricNamesStats } from "../../../api/tsdb";
import { MetricNameStats, TSDBStatus } from "../types";
import AppConfigurator from "../appConfigurator";
import { useSearchParams } from "react-router-dom";
import dayjs from "dayjs";
import { DATE_FORMAT } from "../../../constants/date";
import { getTenantIdFromUrl } from "../../../utils/tenants";
import usePrevious from "../../../hooks/usePrevious";

export const useFetchQuery = (): {
  fetchUrl?: string[],
  isLoading: boolean,
  error?: ErrorTypes | string
  appConfigurator: AppConfigurator,
  isCluster: boolean,
} => {
  const appConfigurator = new AppConfigurator();

  const [searchParams] = useSearchParams();
  const match = searchParams.get("match");
  const focusLabel = searchParams.get("focusLabel");
  const topN = +(searchParams.get("topN") || 10);
  const date = searchParams.get("date") || dayjs().tz().format(DATE_FORMAT);
  const prevDate = usePrevious(date);
  const prevTotal = useRef<{ data: TSDBStatus }>();

  const { serverUrl } = useAppState();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<ErrorTypes | string>();
  const [tsdbStatus, setTSDBStatus] = useState<TSDBStatus>(appConfigurator.defaultTSDBStatus);
  const [metricNamesStats, setMetricNamesStats] = useState<MetricNameStats>(appConfigurator.defaultMetricNameStats);
  const [isCluster, setIsCluster] = useState<boolean>(false);

  const getResponseJson = async (url: string) => {
    const response = await fetch(url);
    const json = await response.json();
    if (response.ok) {
      return json;
    }
    console.error(`Error fetching ${url}:`, json);

    const errorType = json.errorType || ErrorTypes.unknownType;
    const errorMessage = json?.error || json?.message || "see console for more details";
    const error = [errorType, errorMessage].join("\r\n");
    throw new Error(error);
  };

  const calculateDiffs = (result: TSDBStatus, prevResult: TSDBStatus) => {
    Object.keys(result).forEach(k => {
      const key = k as keyof TSDBStatus;
      const entries = result[key];
      const prevEntries = prevResult[key];

      if (Array.isArray(entries) && Array.isArray(prevEntries)) {
        entries.forEach((entry) => {
          const valuePrev = prevEntries.find(prevEntry => prevEntry.name === entry.name)?.value;
          const diff = valuePrev ? entry.value - valuePrev : 0;
          const diffPercent = valuePrev ? (diff / valuePrev) * 100 : 0;
          entry.diff = diff;
          entry.diffPercent = diffPercent;
          entry.valuePrev = valuePrev || 0;
        });
      }
    });
  };

  const fetchCardinalityInfo = async (requestParams: CardinalityRequestsParams) => {
    if (!serverUrl) return;
    setError("");
    setIsLoading(true);
    setTSDBStatus(appConfigurator.defaultTSDBStatus);

    const totalParams = {
      ...requestParams,
      date: requestParams.date,
      topN: 0,
      match: "",
      focusLabel: ""
    };

    const prevDayParams = {
      ...requestParams,
      date: dayjs(requestParams.date).subtract(1, "day").format(DATE_FORMAT),
    };

    const urls = [
      getCardinalityInfo(serverUrl, requestParams),
      getCardinalityInfo(serverUrl, prevDayParams),
    ];

    if (prevDate !== date && (requestParams.match || requestParams.focusLabel)) {
      urls.push(getCardinalityInfo(serverUrl, totalParams));
    }

    try {
      const [resp, respPrev, respTotals] = await Promise.all(urls.map(getResponseJson));

      const prevResult = { ...respPrev.data };
      const { data: dataTotal } = respTotals || prevTotal.current || resp;
      prevTotal.current = { data: dataTotal as TSDBStatus };
      const result: TSDBStatus = {
        ...resp.data,
        totalSeries: resp.data?.totalSeries || resp.data?.headStats?.numSeries || 0,
        totalLabelValuePairs: resp.data?.totalLabelValuePairs || resp.data?.headStats?.numLabelValuePairs || 0,
        seriesCountByLabelName: resp.data?.seriesCountByLabelName || [],
        seriesCountByFocusLabelValue: resp.data?.seriesCountByFocusLabelValue || [],
        totalSeriesByAll: dataTotal?.totalSeries || dataTotal?.headStats?.numSeries || tsdbStatus.totalSeriesByAll || 0,
        totalSeriesPrev: prevResult?.totalSeries || prevResult?.headStats?.numSeries || 0,
      };

      const name = match?.replace(/[{}"]/g, "");
      result.seriesCountByLabelValuePair = result.seriesCountByLabelValuePair.filter(s => s.name !== name);

      calculateDiffs(result, prevResult);

      setTSDBStatus(result);
      setIsLoading(false);
    } catch (e) {
      setIsLoading(false);
      if (e instanceof Error) setError(`${e.name}: ${e.message}`);
    }
  };

  const fetchMetricNamesStats = async () => {
    if (!serverUrl) return;
    setError("");
    setIsLoading(true);

    const url = getMetricNamesStats(serverUrl, { limit: 1 });

    try {
      const resp = await getResponseJson(url);
      setMetricNamesStats(resp);
      setIsLoading(false);
    } catch (e) {
      setIsLoading(false);
      setMetricNamesStats(appConfigurator.defaultMetricNameStats);
      console.error(e);
    }
  };

  useEffect(() => {
    fetchCardinalityInfo({ topN, match, date, focusLabel });
  }, [serverUrl, match, focusLabel, topN, date]);

  useEffect(() => {
    fetchMetricNamesStats();
  }, [serverUrl]);

  useEffect(() => {
    if (error) {
      setTSDBStatus(appConfigurator.defaultTSDBStatus);
      setIsLoading(false);
    }
  }, [error]);

  useEffect(() => {
    const id = getTenantIdFromUrl(serverUrl);
    setIsCluster(!!id);
  }, [serverUrl]);


  appConfigurator.tsdbStatusData = tsdbStatus;
  appConfigurator.metricNameStatsData = metricNamesStats;
  return { isLoading, appConfigurator: appConfigurator, error, isCluster };
};
