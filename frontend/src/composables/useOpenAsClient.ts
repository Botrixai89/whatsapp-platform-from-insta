import { useAuthStore } from '@/stores/auth'
import { useOrganizationsStore } from '@/stores/organizations'
import { toast } from 'vue-sonner'

/**
 * Lets a super admin jump into a client's workspace. Uses the same
 * switch-org flow as the organization switcher so the JWT, WebSocket and
 * media URLs all point at the client org.
 */
export function useOpenAsClient() {
  const authStore = useAuthStore()
  const organizationsStore = useOrganizationsStore()

  async function openAsClient(orgId: string) {
    try {
      await authStore.switchOrg(orgId)
      organizationsStore.selectOrganization(orgId)
      const base = ((window as any).__BASE_PATH__ ?? '').replace(/\/$/, '')
      window.location.href = `${base}/`
    } catch {
      toast.error('Failed to open client workspace')
    }
  }

  return { openAsClient }
}
