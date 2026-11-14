import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { type UIEvent, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type InstallSkillRequest,
  SkillApiError,
  type SkillOwnerMatch,
  type SkillRegistrySearchResult,
  type SkillSearchResponse,
  type SkillSearchSort,
  type SkillSupportItem,
  getPopularSkills,
  getSkills,
  installSkill,
  searchSkills,
} from "@/api/skills"
import { getTools } from "@/api/tools"

import { buildUnavailableToolMessages } from "./tool-support"

const MARKET_SEARCH_LIMIT = 20

interface AmbiguousInstallState {
  slug: string
  registry: string
  version?: string
  installKind?: string
  matches: SkillOwnerMatch[]
}

interface SuspiciousInstallState {
  variables: InstallSkillRequest
  summary?: string
}

export function useSkillsDiscover() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isLoadMoreLockedRef = useRef(false)

  const [marketQuery, setMarketQuery] = useState("")
  const [submittedMarketQuery, setSubmittedMarketQuery] = useState("")
  const [marketSort, setMarketSort] = useState<SkillSearchSort>("relevance")
  const [ambiguousInstall, setAmbiguousInstall] =
    useState<AmbiguousInstallState | null>(null)
  const [suspiciousInstall, setSuspiciousInstall] =
    useState<SuspiciousInstallState | null>(null)

  const { data: skillsData } = useQuery({
    queryKey: ["skills"],
    queryFn: () => getSkills(),
  })
  const { data: toolsData } = useQuery({
    queryKey: ["tools"],
    queryFn: getTools,
  })

  const findSkillsTool = toolsData?.tools.find(
    (tool) => tool.name === "find_skills",
  )
  const installSkillTool = toolsData?.tools.find(
    (tool) => tool.name === "install_skill",
  )
  const canSearchMarketplace = findSkillsTool?.status === "enabled"
  const canInstallFromMarketplace = installSkillTool?.status === "enabled"
  const hasSubmittedQuery = submittedMarketQuery.trim() !== ""
  const isMarketSearchActive = canSearchMarketplace && hasSubmittedQuery

  const {
    data: marketSearchData,
    isPending: isMarketSearchPending,
    isFetching: isMarketSearchFetching,
    isFetchingNextPage,
    error: marketSearchError,
    hasNextPage,
    fetchNextPage,
    refetch: refetchMarketSearch,
  } = useInfiniteQuery({
    queryKey: ["skills-marketplace", submittedMarketQuery, marketSort],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      searchSkills(
        submittedMarketQuery,
        MARKET_SEARCH_LIMIT,
        Number(pageParam) || 0,
        marketSort,
      ),
    getNextPageParam: (lastPage: SkillSearchResponse) =>
      lastPage.has_more ? (lastPage.next_offset ?? undefined) : undefined,
    enabled: isMarketSearchActive,
    staleTime: 5 * 60 * 1000,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  })

  const installMutation = useMutation({
    mutationFn: installSkill,
    onSuccess: (response) => {
      toast.success(
        t("pages.agent.skills.install_success", {
          name: response.skill?.name ?? response.slug,
        }),
      )
      setAmbiguousInstall(null)
      setSuspiciousInstall(null)
      void queryClient.invalidateQueries({ queryKey: ["skills"] })
      void queryClient.invalidateQueries({ queryKey: ["skills-marketplace"] })
      void queryClient.invalidateQueries({ queryKey: ["skills-popular"] })
    },
    onError: (err) => {
      if (
        err instanceof SkillApiError &&
        err.status === 409 &&
        err.code === "suspicious_skill"
      ) {
        const current = installMutation.variables
        if (current) {
          setSuspiciousInstall({
            variables: current,
            summary: err.summary,
          })
          return
        }
      }
      if (
        err instanceof SkillApiError &&
        err.status === 409 &&
        err.code === "ambiguous_slug" &&
        err.matches?.length
      ) {
        const current = installMutation.variables
        if (current) {
          setAmbiguousInstall({
            slug: current.slug,
            registry: current.registry,
            version: current.version,
            installKind: current.install_kind,
            matches: err.matches,
          })
          return
        }
      }
      toast.error(
        err instanceof Error
          ? err.message
          : t("pages.agent.skills.install_error"),
      )
    },
  })

  const {
    data: popularData,
    isPending: isPopularPending,
    isFetching: isPopularFetching,
    error: popularError,
    hasNextPage: hasMorePopularResults,
    fetchNextPage: fetchNextPopularPage,
    isFetchingNextPage: isPopularFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: ["skills-popular"],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      getPopularSkills(MARKET_SEARCH_LIMIT, String(pageParam || "")),
    getNextPageParam: (lastPage: SkillSearchResponse) =>
      lastPage.has_more ? (lastPage.next_cursor ?? undefined) : undefined,
    enabled: canSearchMarketplace,
    staleTime: 60 * 1000,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  })

  const popularResults =
    popularData?.pages.flatMap((page) => page.results) ?? []
  const isPopularInitialLoading =
    canSearchMarketplace &&
    !popularData &&
    (isPopularPending || isPopularFetching)
  const isPopularLoadingMore = Boolean(popularData) && isPopularFetchingNextPage

  const allSkills = skillsData?.skills ?? []
  const marketResults =
    marketSearchData?.pages.flatMap((page) => page.results) ?? []
  const hasMoreMarketResults = hasNextPage ?? false
  const isMarketSearchInitialLoading =
    isMarketSearchActive &&
    !marketSearchData &&
    (isMarketSearchPending || isMarketSearchFetching)
  const isMarketSearchLoadingMore =
    isMarketSearchActive && Boolean(marketSearchData) && isFetchingNextPage
  const installPendingKey =
    installMutation.isPending && installMutation.variables
      ? `${installMutation.variables.registry}:${installMutation.variables.slug}`
      : null

  const unavailableToolMessages = buildUnavailableToolMessages({
    searchTool: findSkillsTool,
    installTool: installSkillTool,
    t,
  })

  useEffect(() => {
    if (!isFetchingNextPage && !isPopularFetchingNextPage) {
      isLoadMoreLockedRef.current = false
    }
  }, [isFetchingNextPage, isPopularFetchingNextPage])

  const handleSearchSubmit = () => {
    const nextQuery = marketQuery.trim()
    if (!canSearchMarketplace || nextQuery === "") {
      return
    }

    isLoadMoreLockedRef.current = false
    if (nextQuery === submittedMarketQuery) {
      void refetchMarketSearch()
      return
    }

    setSubmittedMarketQuery(nextQuery)
  }

  const handleInstall = (result: SkillRegistrySearchResult) => {
    installMutation.mutate({
      slug: result.slug,
      registry: result.registry_name,
      version: result.version || undefined,
      owner_handle: result.owner_handle || undefined,
      install_kind: result.install_kind || undefined,
    })
  }

  const handleInstallWithOwner = (ownerHandle: string) => {
    const pending = ambiguousInstall
    if (!pending) {
      return
    }
    setAmbiguousInstall(null)
    installMutation.mutate({
      slug: pending.slug,
      registry: pending.registry,
      version: pending.version,
      owner_handle: ownerHandle,
      install_kind: pending.installKind,
    })
  }

  const handleConfirmSuspicious = () => {
    const pending = suspiciousInstall
    if (!pending) {
      return
    }
    setSuspiciousInstall(null)
    installMutation.mutate({ ...pending.variables, accept_suspicious: true })
  }

  const handleSortChange = (nextSort: SkillSearchSort) => {
    if (nextSort === marketSort) {
      return
    }
    isLoadMoreLockedRef.current = false
    setMarketSort(nextSort)
  }

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    const node = event.currentTarget
    const remaining = node.scrollHeight - node.scrollTop - node.clientHeight
    if (remaining > 240) {
      return
    }
    if (isLoadMoreLockedRef.current) {
      return
    }

    if (!hasSubmittedQuery) {
      if (!hasMorePopularResults || isPopularFetchingNextPage) {
        return
      }
      isLoadMoreLockedRef.current = true
      void fetchNextPopularPage()
      return
    }

    if (!isMarketSearchActive || !hasMoreMarketResults || isFetchingNextPage) {
      return
    }

    isLoadMoreLockedRef.current = true
    void fetchNextPage()
  }

  const getInstalledSkill = (
    installedId?: string,
    installedName?: string,
  ): SkillSupportItem | null => {
    if (installedId) {
      const byID = allSkills.find((skill) => skill.id === installedId)
      if (byID) {
        return byID
      }
    }
    if (installedName) {
      return allSkills.find((skill) => skill.name === installedName) ?? null
    }
    return null
  }

  const isInstallPending = (result: SkillRegistrySearchResult) =>
    installPendingKey === `${result.registry_name}:${result.slug}`

  return {
    marketQuery,
    submittedMarketQuery,
    canSearchMarketplace,
    canInstallFromMarketplace,
    marketResults,
    marketSearchError,
    popularResults,
    popularError,
    hasMorePopularResults,
    isPopularInitialLoading,
    isPopularLoadingMore,
    unavailableToolMessages,
    hasSubmittedQuery,
    isMarketSearchInitialLoading,
    isMarketSearchLoadingMore,
    marketSort,
    ambiguousInstall,
    suspiciousInstall,
    setMarketQuery,
    handleSearchSubmit,
    handleInstall,
    handleInstallWithOwner,
    handleConfirmSuspicious,
    handleSortChange,
    clearAmbiguousInstall: () => setAmbiguousInstall(null),
    clearSuspiciousInstall: () => setSuspiciousInstall(null),
    handleScroll,
    getInstalledSkill,
    isInstallPending,
  }
}
