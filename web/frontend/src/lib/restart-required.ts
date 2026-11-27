import type { TFunction } from "i18next"
import { toast } from "sonner"

import { restartGateway } from "@/api/gateway"
import { restartLauncher } from "@/api/system"
import { refreshGatewayState } from "@/store"

export function showRestartRequiredToast(t: TFunction, name: string) {
  toast.warning(t("common.restartRequiredTitle"), {
    description: t("common.restartRequiredDesc", { name }),
    duration: 15000,
    action: {
      label: t("common.restartNow", "Restart now"),
      onClick: () => {
        void restartLauncher()
          .then(() => {
            toast.info(
              t(
                "pages.config.launcher_restarting",
                "PicoClaw is restarting. The page refreshes automatically.",
              ),
            )
            window.setTimeout(() => window.location.reload(), 6000)
          })
          .catch((error) => {
            toast.error(
              error instanceof Error
                ? error.message
                : t("common.restartFailed", "Could not restart PicoClaw."),
            )
          })
      },
    },
  })
}

export function showGatewayRestartRequiredToast(t: TFunction, name: string) {
  toast.warning(t("common.restartRequiredTitle", "Gateway restart required"), {
    description: t(
      "common.restartRequiredDesc",
      "The latest {{name}} configuration has been saved. Restart the gateway for it to take effect.",
      { name },
    ),
    duration: 20000,
    action: {
      label: t("common.restartNow", "Restart now"),
      onClick: () => {
        void restartGateway()
          .then(async () => {
            await refreshGatewayState({ force: true })
            toast.info(
              t(
                "common.gatewayRestarting",
                "Gateway restarted. The new settings are active.",
              ),
            )
          })
          .catch((error) => {
            toast.error(
              error instanceof Error
                ? error.message
                : t(
                    "common.gatewayRestartFailed",
                    "Could not restart the gateway.",
                  ),
            )
          })
      },
    },
  })
}

export function showSaveSuccessOrRestartToast(
  t: TFunction,
  savedMessage: string,
  name: string,
  restartRequired: boolean,
) {
  if (restartRequired) {
    showRestartRequiredToast(t, name)
    return
  }
  toast.success(savedMessage)
}
