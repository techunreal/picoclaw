import { atom } from "jotai"
import { atomWithStorage } from "jotai/utils"

export type TourStep =
  | "welcome"
  | "models"
  | "gateway"
  | "agentTools"
  | "services"
  | "usage"
  | "docs"
  | "completed"

// TOUR_STEP_ORDER drives next/previous navigation and the progress counter.
export const TOUR_STEP_ORDER: TourStep[] = [
  "welcome",
  "models",
  "gateway",
  "agentTools",
  "services",
  "usage",
  "docs",
  "completed",
]

export interface TourState {
  currentStep: TourStep
  isActive: boolean
}

const STORAGE_KEY = "picoclaw-tour-state"

const DEFAULT_TOUR_STATE: TourState = {
  currentStep: "welcome",
  isActive: true,
}

export const tourAtom = atomWithStorage<TourState>(
  STORAGE_KEY,
  DEFAULT_TOUR_STATE,
)

export const tourIsActiveAtom = atom(
  (get) => get(tourAtom).isActive,
  (get, set, isActive: boolean) => {
    set(tourAtom, { ...get(tourAtom), isActive })
  },
)

export const tourCurrentStepAtom = atom(
  (get) => get(tourAtom).currentStep,
  (get, set, step: TourStep) => {
    set(tourAtom, { ...get(tourAtom), currentStep: step })
  },
)

export function useTourActions() {
  const goToNextStep = (currentStep: TourStep): TourStep => {
    const currentIndex = TOUR_STEP_ORDER.indexOf(currentStep)
    if (currentIndex < TOUR_STEP_ORDER.length - 1) {
      return TOUR_STEP_ORDER[currentIndex + 1]
    }
    return "completed"
  }

  const goToPrevStep = (currentStep: TourStep): TourStep => {
    const currentIndex = TOUR_STEP_ORDER.indexOf(currentStep)
    if (currentIndex > 0) {
      return TOUR_STEP_ORDER[currentIndex - 1]
    }
    return currentStep
  }

  return { goToNextStep, goToPrevStep }
}
