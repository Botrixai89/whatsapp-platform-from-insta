import { ref } from 'vue'

// Shared so the sidebar's Help item and the floating launcher open the same panel
const isOpen = ref(false)

export function useHelpAssistant() {
  return {
    isOpen,
    open: () => { isOpen.value = true },
    close: () => { isOpen.value = false },
    toggle: () => { isOpen.value = !isOpen.value },
  }
}
