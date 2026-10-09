import { describe, it, expect } from 'vitest'
import { DESTINATIONS, FAQS, matchLocally, suggestionsFor } from './assistant'

const all = DESTINATIONS
const clientOnly = DESTINATIONS.filter(d => !d.superAdminOnly)

describe('matchLocally', () => {
  it('navigates on short or "open X" requests', () => {
    expect(matchLocally('open wallet', all).navigateTo?.id).toBe('wallet')
    expect(matchLocally('templates', all).navigateTo?.id).toBe('templates')
    expect(matchLocally('template page pe le chalo', all).navigateTo?.id).toBe('templates')
  })

  it('answers how-to questions from the FAQ', () => {
    expect(matchLocally('How do I connect my WhatsApp number?', all).faq?.id).toBe('connect_number')
    expect(matchLocally('why was my template rejected', all).faq?.id).toBe('template_rejected')
    expect(matchLocally('24 hour window kya hai', all).faq?.id).toBe('window_24h')
  })

  it('understands Hinglish', () => {
    const m = matchLocally('wallet me paise kaise add kare', all)
    expect(m.faq?.id ?? m.weakFaq?.id).toBe('recharge')
    expect(matchLocally('naya template banao', all).faq?.id).toBe('create_template')
  })

  it('maps every suggestion chip to its own answer', () => {
    for (const f of FAQS) {
      expect(matchLocally(f.question, all).faq?.id, f.question).toBe(f.id)
    }
  })

  it('never offers pages the user cannot open', () => {
    const m = matchLocally('open clients', clientOnly)
    expect(m.navigateTo).toBeUndefined()
    expect(m.restricted?.id).toBe('admin_clients')
    expect(m.destinations.some(d => d.superAdminOnly)).toBe(false)
    const recharge = matchLocally('How do I recharge my wallet?', clientOnly).faq!
    expect(recharge.links).toEqual(['wallet'])
  })

  it('does not guess on unrelated text', () => {
    const m = matchLocally('what is the weather today', all)
    expect(m.confident).toBe(false)
  })
})

describe('suggestionsFor', () => {
  it('uses page-specific questions', () => {
    expect(suggestionsFor('/chat/123')).toContain(FAQS.find(f => f.id === 'window_24h')!.question)
    expect(suggestionsFor('/nowhere').length).toBeGreaterThan(0)
  })
})
