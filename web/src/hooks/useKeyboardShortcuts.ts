import { useEffect } from 'react'

interface Shortcuts {
  onArrowLeft?: () => void
  onArrowRight?: () => void
  onArrowUp?: () => void
  onArrowDown?: () => void
  onEnter?: () => void
  onSpace?: () => void
  onF?: () => void
  onEscape?: () => void
  onM?: () => void
  onDigit1?: () => void
  onDigit2?: () => void
  onDigit3?: () => void
  onQuestion?: () => void
}

export function useKeyboardShortcuts(shortcuts: Shortcuts, enabled: boolean = true) {
  useEffect(() => {
    if (!enabled) return

    function handler(e: KeyboardEvent) {
      const target = e.target as HTMLElement
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return

      switch (e.code) {
        case 'ArrowLeft': e.preventDefault(); shortcuts.onArrowLeft?.(); break
        case 'ArrowRight': e.preventDefault(); shortcuts.onArrowRight?.(); break
        case 'ArrowUp': e.preventDefault(); shortcuts.onArrowUp?.(); break
        case 'ArrowDown': e.preventDefault(); shortcuts.onArrowDown?.(); break
        case 'Enter': e.preventDefault(); shortcuts.onEnter?.(); break
        case 'Space': e.preventDefault(); shortcuts.onSpace?.(); break
        case 'KeyF': e.preventDefault(); shortcuts.onF?.(); break
        case 'Escape': e.preventDefault(); shortcuts.onEscape?.(); break
        case 'KeyM': e.preventDefault(); shortcuts.onM?.(); break
        case 'Digit1': e.preventDefault(); shortcuts.onDigit1?.(); break
        case 'Digit2': e.preventDefault(); shortcuts.onDigit2?.(); break
        case 'Digit3': e.preventDefault(); shortcuts.onDigit3?.(); break
        case 'Slash': e.preventDefault(); shortcuts.onQuestion?.(); break
      }
    }

    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [shortcuts, enabled])
}
