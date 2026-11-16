import {
  IconBook,
  IconChevronLeft,
  IconChevronRight,
} from "@tabler/icons-react"
import { useRouterState } from "@tanstack/react-router"
import { useAtom } from "jotai"
import { useEffect, useLayoutEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import {
  TOUR_STEP_ORDER,
  type TourStep,
  tourAtom,
  tourCurrentStepAtom,
  tourIsActiveAtom,
  useTourActions,
} from "@/store/tour"

interface TourStepConfig {
  title: string
  description: string
  targetSelector?: string
  position: "top" | "bottom" | "left" | "right"
  icon?: React.ReactNode
  offsetY?: number
}

export function TourGuide() {
  const { t } = useTranslation()
  const [tourState] = useAtom(tourAtom)
  const [, setCurrentStep] = useAtom(tourCurrentStepAtom)
  const [, setIsActive] = useAtom(tourIsActiveAtom)
  const { goToNextStep, goToPrevStep } = useTourActions()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const tourStartPathRef = useRef(pathname)

  // End the tour when the user navigates to another page, so its decorative
  // backdrop never lingers over a page it was not opened on.
  useEffect(() => {
    if (!tourState.isActive) {
      tourStartPathRef.current = pathname
      return
    }
    if (pathname !== tourStartPathRef.current) {
      tourStartPathRef.current = pathname
      setCurrentStep("completed")
      setIsActive(false)
    }
  }, [pathname, tourState.isActive, setCurrentStep, setIsActive])

  const steps: Record<TourStep, TourStepConfig> = {
    welcome: {
      title: t("tour.welcome.title"),
      description: t("tour.welcome.description"),
      position: "bottom",
    },
    chat: {
      title: t("tour.chat.title"),
      description: t("tour.chat.description"),
      targetSelector: "[data-tour='chat-nav']",
      position: "right",
    },
    agents: {
      title: t("tour.agents.title"),
      description: t("tour.agents.description"),
      targetSelector: "[data-tour='agent-group']",
      position: "right",
    },
    settings: {
      title: t("tour.settings.title"),
      description: t("tour.settings.description"),
      targetSelector: "[data-tour='settings-gear']",
      position: "right",
    },
    gateway: {
      title: t("tour.gateway.title"),
      description: t("tour.gateway.description"),
      targetSelector: "[data-tour='gateway-button']",
      position: "left",
      offsetY: 60,
    },
    composer: {
      title: t("tour.composer.title"),
      description: t("tour.composer.description"),
      targetSelector: "[data-tour='composer']",
      position: "top",
    },
    docs: {
      title: t("tour.docs.title"),
      description: t("tour.docs.description"),
      targetSelector: "[data-tour='docs-button']",
      position: "left",
      icon: <IconBook className="size-4" />,
      offsetY: 60,
    },
    completed: {
      title: "",
      description: "",
      position: "bottom",
    },
  }

  const currentConfig = steps[tourState.currentStep]
  const currentStepIndex = TOUR_STEP_ORDER.indexOf(tourState.currentStep)
  const totalSteps = TOUR_STEP_ORDER.length - 1

  const handleNext = () => {
    const nextStep = goToNextStep(tourState.currentStep)
    setCurrentStep(nextStep)
    if (nextStep === "completed") {
      setIsActive(false)
    }
  }

  const handlePrev = () => {
    const prevStep = goToPrevStep(tourState.currentStep)
    setCurrentStep(prevStep)
  }

  const handleSkip = () => {
    setCurrentStep("completed")
    setIsActive(false)
  }

  const getTargetElement = () => {
    if (!currentConfig.targetSelector) return null
    return document.querySelector(currentConfig.targetSelector)
  }

  const targetElement = getTargetElement()

  const getPopoverPosition = () => {
    if (!targetElement) {
      return {
        top: "50%",
        left: "50%",
        transform: "translate(-50%, -50%)",
      }
    }

    const rect = targetElement.getBoundingClientRect()
    const offset = 12
    const offsetY = currentConfig.offsetY ?? 0

    switch (currentConfig.position) {
      case "top":
        return {
          top: rect.top - offset,
          left: rect.left + rect.width / 2,
          transform: "translate(-50%, -100%)",
        }
      case "bottom":
        return {
          top: rect.bottom + offset,
          left: rect.left + rect.width / 2,
          transform: "translateX(-50%)",
        }
      case "left":
        return {
          top: rect.top + rect.height / 2 + offsetY,
          left: rect.left - offset,
          transform: "translate(-100%, -50%)",
        }
      case "right":
        return {
          top: rect.top + rect.height / 2 + offsetY,
          left: rect.right + offset,
          transform: "translateY(-50%)",
        }
      default:
        return {
          top: rect.bottom + offset,
          left: rect.left + rect.width / 2,
          transform: "translateX(-50%)",
        }
    }
  }

  const position = getPopoverPosition()
  const isCentered = !targetElement

  // Nudge the popover back inside the viewport when it would be clipped
  // (e.g. a sidebar group near the bottom of the window).
  const popoverRef = useRef<HTMLDivElement>(null)
  const [popoverShift, setPopoverShift] = useState({ x: 0, y: 0 })
  useLayoutEffect(() => {
    setPopoverShift({ x: 0, y: 0 })
    const frame = requestAnimationFrame(() => {
      const node = popoverRef.current
      if (!node) return
      const rect = node.getBoundingClientRect()
      const margin = 8
      let x = 0
      let y = 0
      if (rect.right > window.innerWidth - margin) {
        x = window.innerWidth - margin - rect.right
      }
      if (rect.left < margin) {
        x = margin - rect.left
      }
      if (rect.bottom > window.innerHeight - margin) {
        y = window.innerHeight - margin - rect.bottom
      }
      if (rect.top < margin) {
        y = margin - rect.top
      }
      if (x !== 0 || y !== 0) {
        setPopoverShift({ x, y })
      }
    })
    return () => cancelAnimationFrame(frame)
  }, [tourState.currentStep, targetElement])

  if (!tourState.isActive || tourState.currentStep === "completed") {
    return null
  }

  return (
    <>
      {targetElement ? (
        <div
          className="pointer-events-none fixed z-[100] transition-all duration-300"
          style={{
            top: targetElement.getBoundingClientRect().top - 8,
            left: targetElement.getBoundingClientRect().left - 8,
            width: targetElement.getBoundingClientRect().width + 16,
            height: targetElement.getBoundingClientRect().height + 16,
            boxShadow:
              "0 0 0 9999px rgba(0, 0, 0, 0.2), 0 0 2px 9999px rgba(0, 0, 0, 0.1)",
            borderRadius: "12px",
          }}
        />
      ) : (
        <div className="pointer-events-none fixed inset-0 z-[100] bg-black/20 backdrop-blur-[2px]" />
      )}

      {targetElement && (
        <div
          className="ring-primary ring-offset-background pointer-events-none fixed z-[101] rounded-lg ring-2 ring-offset-2 transition-all duration-300"
          style={{
            top: targetElement.getBoundingClientRect().top - 4,
            left: targetElement.getBoundingClientRect().left - 4,
            width: targetElement.getBoundingClientRect().width + 8,
            height: targetElement.getBoundingClientRect().height + 8,
          }}
        />
      )}

      <div
        ref={popoverRef}
        className={cn(
          "bg-background fixed z-[102] w-80 max-h-[calc(100vh-1rem)] overflow-y-auto rounded-xl border p-4 shadow-2xl",
          isCentered && "max-w-md",
        )}
        style={{
          ...position,
          transform:
            typeof position.transform === "string"
              ? `${position.transform} translate(${popoverShift.x}px, ${popoverShift.y}px)`
              : `translate(${popoverShift.x}px, ${popoverShift.y}px)`,
        }}
      >
        <div className="mb-3 flex items-center gap-2">
          {currentConfig.icon}
          <h3 className="font-semibold">{currentConfig.title}</h3>
        </div>

        <p className="text-muted-foreground mb-4 text-sm leading-relaxed">
          {currentConfig.description}
        </p>

        <div className="flex items-center justify-between">
          <div className="text-muted-foreground text-xs">
            {currentStepIndex + 1} / {totalSteps}
          </div>

          <div className="flex items-center gap-2">
            {currentStepIndex > 0 && (
              <Button variant="outline" size="sm" onClick={handlePrev}>
                <IconChevronLeft className="size-4" />
                {t("tour.prev")}
              </Button>
            )}
            <Button size="sm" onClick={handleNext}>
              {currentStepIndex === totalSteps - 1
                ? t("tour.finish")
                : t("tour.next")}
              {currentStepIndex < totalSteps - 1 && (
                <IconChevronRight className="size-4" />
              )}
            </Button>
          </div>
        </div>

        {currentStepIndex < totalSteps - 1 && (
          <Button
            variant="link"
            size="sm"
            className="mt-2 h-auto p-0 text-xs"
            onClick={handleSkip}
          >
            {t("tour.skip")}
          </Button>
        )}
      </div>
    </>
  )
}
