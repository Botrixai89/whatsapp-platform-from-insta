/**
 * Built-in knowledge for the in-app help assistant: where things live and how
 * to do common tasks. Answers come from here first (instant, free, offline);
 * the AI endpoint is only asked when nothing here matches well.
 */

export interface AssistantDestination {
  id: string
  title: string
  path: string
  description: string
  /** Same permission keys as the sidebar (read access) */
  permission?: string
  superAdminOnly?: boolean
  /** Lower-case words/phrases, English and Hinglish */
  keywords: string[]
}

export interface AssistantFaq {
  id: string
  /** Shown as a suggestion chip */
  question: string
  keywords: string[]
  answer: string
  /** Extra sentence shown only to platform owners */
  ownerNote?: string
  links: string[]
}

export const DESTINATIONS: AssistantDestination[] = [
  // Owner panel
  { id: 'owner_dashboard', title: 'Owner Dashboard', path: '/admin', superAdminOnly: true, description: 'Platform-wide revenue, clients and message volume', keywords: ['owner', 'platform', 'revenue', 'kamai', 'owner dashboard', 'admin panel', 'super admin'] },
  { id: 'admin_clients', title: 'Clients', path: '/admin/clients', superAdminOnly: true, description: 'All client businesses, their balance, plan and status; add a client, add or deduct funds, suspend', keywords: ['client', 'clients', 'customer business', 'add client', 'suspend', 'add funds', 'credit', 'deduct', 'customers list'] },
  { id: 'admin_plans', title: 'Plans', path: '/admin/plans', superAdminOnly: true, description: 'Subscription plans and their limits', keywords: ['plan', 'plans', 'subscription', 'limit', 'pricing plan', 'package'] },
  { id: 'admin_rates', title: 'Rate Card', path: '/admin/rates', superAdminOnly: true, description: 'Per-message prices charged to clients by country and category', keywords: ['rate', 'rates', 'rate card', 'price per message', 'message cost', 'markup', 'charges'] },
  { id: 'admin_transactions', title: 'All Transactions', path: '/admin/transactions', superAdminOnly: true, description: 'Wallet ledger across all clients', keywords: ['transactions', 'ledger', 'all payments', 'recharge history'] },

  // Main
  { id: 'dashboard', title: 'Dashboard', path: '/', permission: 'analytics', description: 'Overview widgets for messages, contacts and campaigns', keywords: ['dashboard', 'home', 'overview', 'stats', 'widget'] },
  { id: 'chat', title: 'Chat inbox', path: '/chat', permission: 'chat', description: 'Reply to customers, filter Open/Closed/Unread chats, assign chats, turn the bot off per chat', keywords: ['chat', 'chats', 'inbox', 'conversation', 'reply', 'message customer', 'baat', 'jawab', 'customer messages'] },
  { id: 'wallet', title: 'Wallet', path: '/wallet', permission: 'settings.general', description: 'Balance, message rates, spend by category and transaction history', keywords: ['wallet', 'balance', 'recharge', 'top up', 'topup', 'add money', 'paise', 'paisa', 'credit', 'spend', 'kharcha', 'bill', 'billing', 'charges', 'transaction'] },

  // Messaging
  { id: 'chatbot', title: 'Chatbot overview', path: '/chatbot', permission: 'settings.chatbot', description: 'Chatbot status and shortcuts to keywords, flows and AI', keywords: ['chatbot', 'bot', 'automation', 'auto reply', 'autoreply'] },
  { id: 'chatbot_keywords', title: 'Keyword rules', path: '/chatbot/keywords', permission: 'chatbot.keywords', description: 'Automatic replies when a message contains a keyword', keywords: ['keyword', 'keywords', 'auto reply', 'trigger word'] },
  { id: 'chatbot_keyword_new', title: 'New keyword rule', path: '/chatbot/keywords/new', permission: 'chatbot.keywords', description: 'Create an automatic reply for a keyword', keywords: ['new keyword', 'add keyword', 'create keyword'] },
  { id: 'chatbot_flows', title: 'Chatbot flows', path: '/chatbot/flows', permission: 'flows.chatbot', description: 'Visual multi-step conversation flows', keywords: ['flow', 'flows', 'bot flow', 'conversation flow', 'flow builder'] },
  { id: 'chatbot_flow_new', title: 'New chatbot flow', path: '/chatbot/flows/new', permission: 'flows.chatbot', description: 'Build a new conversation flow', keywords: ['new flow', 'create flow', 'build flow', 'flow banao'] },
  { id: 'chatbot_ai', title: 'AI contexts', path: '/chatbot/ai', permission: 'chatbot.ai', description: 'Knowledge the AI uses to reply to customers', keywords: ['ai context', 'knowledge base', 'ai reply', 'gpt', 'ai bot'] },
  { id: 'chatbot_transfers', title: 'Agent transfers', path: '/chatbot/transfers', permission: 'transfers', description: 'Chats handed from the bot to human agents', keywords: ['transfer', 'transfers', 'human agent', 'handover', 'queue'] },
  { id: 'campaigns', title: 'Campaigns', path: '/campaigns', permission: 'campaigns', description: 'Bulk template messages to many contacts', keywords: ['campaign', 'campaigns', 'broadcast', 'bulk', 'mass message', 'blast'] },
  { id: 'campaign_new', title: 'New campaign', path: '/campaigns/new', permission: 'campaigns', description: 'Create a bulk send from an approved template', keywords: ['new campaign', 'create campaign', 'send bulk', 'broadcast bhejo', 'campaign banao'] },
  { id: 'templates', title: 'Templates', path: '/templates', permission: 'templates', description: 'WhatsApp message templates and their Meta approval status', keywords: ['template', 'templates', 'approval', 'approved', 'rejected', 'pending'] },
  { id: 'template_new', title: 'New template', path: '/templates/new', permission: 'templates', description: 'Create a template manually with live preview', keywords: ['new template', 'create template', 'add template', 'template banao', 'make template'] },
  { id: 'template_ai', title: 'Create template with AI', path: '/templates/new?ai=1', permission: 'templates', description: 'Describe a message and get ready-to-submit templates', keywords: ['ai template', 'template with ai', 'generate template', 'write template'] },
  { id: 'template_analytics', title: 'Template analytics', path: '/templates/analytics', permission: 'templates', description: 'Sent, delivered, read, failed and spend per template', keywords: ['template analytics', 'template report', 'read rate', 'delivery rate', 'template performance'] },
  { id: 'whatsapp_flows', title: 'WhatsApp Flows', path: '/flows', permission: 'flows.whatsapp', description: 'Native WhatsApp forms', keywords: ['whatsapp flow', 'form', 'forms'] },

  // Calling & analytics
  { id: 'call_logs', title: 'Call logs', path: '/calling/logs', permission: 'call_logs', description: 'WhatsApp call history', keywords: ['call', 'calls', 'call log', 'call history', 'missed call'] },
  { id: 'ivr_flows', title: 'IVR flows', path: '/calling/ivr-flows', permission: 'ivr_flows', description: 'Phone menu flows for WhatsApp calls', keywords: ['ivr', 'phone menu', 'press 1'] },
  { id: 'call_transfers', title: 'Call transfers', path: '/calling/transfers', permission: 'call_transfers', description: 'Calls waiting for an agent', keywords: ['call transfer', 'call queue'] },
  { id: 'agent_analytics', title: 'Agent analytics', path: '/analytics/agents', permission: 'analytics.agents', description: 'Agent performance, resolution and queue times', keywords: ['agent analytics', 'agent performance', 'team performance', 'resolution time'] },
  { id: 'meta_insights', title: 'Meta insights', path: '/analytics/meta-insights', permission: 'analytics', description: 'Analytics and pricing data from Meta', keywords: ['insights', 'meta insights', 'analytics', 'report', 'reports', 'conversation analytics'] },

  // Settings
  { id: 'settings', title: 'General settings', path: '/settings', permission: 'settings.general', description: 'Organization name, timezone, language, phone masking, Meta app credentials', keywords: ['settings', 'timezone', 'language', 'organization name', 'mask', 'setting'] },
  { id: 'settings_chatbot', title: 'Chatbot settings', path: '/settings/chatbot', permission: 'settings.chatbot', description: 'Greeting, fallback, business hours, AI provider and SLA', keywords: ['chatbot settings', 'greeting', 'welcome message', 'fallback', 'business hours', 'ai provider', 'api key ai', 'sla'] },
  { id: 'accounts', title: 'WhatsApp accounts', path: '/settings/accounts', permission: 'accounts', description: 'Connect WhatsApp numbers (Embedded Signup) and manage tokens', keywords: ['account', 'accounts', 'whatsapp number', 'phone number', 'connect', 'embedded signup', 'waba', 'token', 'number add', 'number jodo'] },
  { id: 'contacts', title: 'Contacts', path: '/settings/contacts', permission: 'contacts', description: 'All contacts; import or export CSV', keywords: ['contact', 'contacts', 'import', 'export', 'csv', 'upload contacts'] },
  { id: 'canned_responses', title: 'Canned responses', path: '/settings/canned-responses', permission: 'canned_responses', description: 'Saved quick replies for agents', keywords: ['canned', 'quick reply', 'saved reply', 'shortcut reply'] },
  { id: 'tags', title: 'Tags', path: '/settings/tags', permission: 'tags', description: 'Labels for contacts and chats', keywords: ['tag', 'tags', 'label', 'labels'] },
  { id: 'teams', title: 'Teams', path: '/settings/teams', permission: 'teams', description: 'Agent teams and assignment rules', keywords: ['team', 'teams', 'department', 'round robin'] },
  { id: 'users', title: 'Users', path: '/settings/users', permission: 'users', description: 'Invite teammates and manage their roles', keywords: ['user', 'users', 'agent', 'agents', 'invite', 'teammate', 'staff', 'member', 'add user', 'employee'] },
  { id: 'roles', title: 'Roles', path: '/settings/roles', permission: 'roles', description: 'Permissions for each role', keywords: ['role', 'roles', 'permission', 'permissions', 'access'] },
  { id: 'api_keys', title: 'API keys', path: '/settings/api-keys', permission: 'api_keys', description: 'Keys for the REST API', keywords: ['api key', 'api keys', 'api', 'developer', 'integration'] },
  { id: 'webhooks', title: 'Webhooks', path: '/settings/webhooks', permission: 'webhooks', description: 'Send platform events to your server', keywords: ['webhook', 'webhooks', 'callback url', 'events'] },
  { id: 'custom_actions', title: 'Custom actions', path: '/settings/custom-actions', permission: 'custom_actions', description: 'Buttons in the chat header that call your systems', keywords: ['custom action', 'crm button', 'action button'] },
  { id: 'sso', title: 'Single sign-on', path: '/settings/sso', permission: 'settings.sso', description: 'Google / Microsoft / OIDC login', keywords: ['sso', 'single sign on', 'google login', 'oauth login'] },
  { id: 'audit_logs', title: 'Audit logs', path: '/settings/audit-logs', permission: 'audit_logs', description: 'Who changed what and when', keywords: ['audit', 'audit log', 'history', 'activity log'] },
  { id: 'profile', title: 'My profile', path: '/profile', description: 'Your account details and password', keywords: ['profile', 'password', 'change password', 'my account'] },
]

export const FAQS: AssistantFaq[] = [
  {
    id: 'connect_number', question: 'How do I connect my WhatsApp number?',
    keywords: ['connect number', 'connect whatsapp', 'add number', 'embedded signup', 'number connect', 'number kaise jode', 'whatsapp number add', 'setup whatsapp', 'onboard'],
    answer: 'Go to **Settings → Accounts** and click **Add Account**. Use **Connect with Facebook** (Embedded Signup) to log in with Meta and pick your business and number, or enter the Phone Number ID, WABA ID and access token manually.',
    links: ['accounts'],
  },
  {
    id: 'recharge', question: 'How do I recharge my wallet?',
    keywords: ['recharge', 'top up', 'topup', 'add money', 'add balance', 'paise dalne', 'paise add', 'balance kam', 'low balance', 'balance low', 'add funds'],
    answer: 'Your wallet is prepaid and is topped up by your account manager. Contact them to add funds; the new balance shows up on the **Wallet** page instantly.',
    ownerNote: 'As the platform owner, you add funds from **Owner Panel → Clients → Add funds**, or **Add funds** on your own Wallet page.',
    links: ['wallet', 'admin_clients'],
  },
  {
    id: 'billing', question: 'How are messages charged?',
    keywords: ['charge', 'charged', 'cost', 'price', 'pricing', 'kitna', 'deduct', 'deduction', 'per message', 'rate', 'kharcha', 'bill'],
    answer: 'Each billable message is charged from your wallet the moment it is sent, using the rates on the **Wallet** page (by country and category, e.g. Marketing or Utility). If WhatsApp later reports the message failed or free, the charge is refunded automatically.',
    links: ['wallet'],
  },
  {
    id: 'window_24h', question: 'Why can\'t I reply to a customer?',
    keywords: ['24 hour', '24h', '24 ghante', 'session expired', 'cannot reply', "can't reply", 'reply nahi', 'window', 'locked', 'send template only'],
    answer: 'WhatsApp only allows free-form replies within **24 hours** of the customer\'s last message. After that the chat is locked and you can only send an approved **template**. Click **Send Template** in the chat; once the customer replies, the window opens again.',
    links: ['chat', 'templates'],
  },
  {
    id: 'message_failed', question: 'Why did my message fail?',
    keywords: ['failed', 'fail', 'not delivered', 'not sent', 'error', 'error 190', 'oauth', 'nahi gaya', 'deliver nahi', 'message nahi ja raha'],
    answer: 'Hover **Not delivered** under the message to see the reason. Most common: **access token expired** (reconnect the number in Settings → Accounts), **24-hour window closed** (send a template), or **wallet balance too low**.',
    links: ['accounts', 'wallet', 'chat'],
  },
  {
    id: 'create_template', question: 'How do I create a template?',
    keywords: ['create template', 'new template', 'template banao', 'template kaise', 'make template', 'add template'],
    answer: 'Open **Templates → Create Template**, fill the body (use {{1}} or {{name}} for variables), add sample values and buttons, and watch the live preview. Or click **Create with AI**, describe your message and pick a variation. Then submit it to Meta for approval.',
    links: ['template_new', 'template_ai'],
  },
  {
    id: 'template_rejected', question: 'Why was my template rejected?',
    keywords: ['rejected', 'reject', 'not approved', 'approval', 'pending', 'review', 'reject ho gaya'],
    answer: 'Meta usually rejects templates for: variables at the very start/end, missing sample values, promotional text in a Utility template, misleading or policy-breaking content, or duplicate content. Fix the text on the template page and resubmit. Approval normally takes minutes to a few hours.',
    links: ['templates'],
  },
  {
    id: 'campaign', question: 'How do I send a bulk campaign?',
    keywords: ['campaign', 'bulk', 'broadcast', 'mass message', 'send to all', 'sabko bhejo', 'bulk message'],
    answer: 'Go to **Campaigns → Create Campaign**, choose an approved template and WhatsApp number, upload your contacts (CSV) and start or schedule it. Keep enough wallet balance: campaigns pause automatically if funds run out.',
    links: ['campaign_new', 'campaigns'],
  },
  {
    id: 'chatbot_setup', question: 'How do I set up a chatbot?',
    keywords: ['chatbot', 'bot setup', 'auto reply', 'autoreply', 'bot banao', 'automation', 'welcome message', 'greeting'],
    answer: 'Start in **Chatbot settings** (greeting, fallback, business hours, AI provider). Then add **Keyword rules** for simple auto-replies, build **Flows** for step-by-step conversations, and add **AI contexts** if you want AI answers from your own knowledge.',
    links: ['settings_chatbot', 'chatbot_keywords', 'chatbot_flows'],
  },
  {
    id: 'bot_off_chat', question: 'How do I stop the bot for one customer?',
    keywords: ['bot off', 'stop bot', 'pause bot', 'disable bot', 'bot band', 'human take over', 'takeover'],
    answer: 'Open the chat and click the **bot icon** in the chat header (it turns amber with a slash). The bot stops replying to that customer only. Click it again to turn it back on. The **Bot off** chip in the chat list shows all such chats.',
    links: ['chat'],
  },
  {
    id: 'close_chat', question: 'How do Open and Closed chats work?',
    keywords: ['close chat', 'closed chat', 'resolve', 'done', 'open chat', 'reopen', 'chat band'],
    answer: 'Click **Close chat** in the chat header when you are done. It moves to the **Closed** chip and reopens automatically when the customer writes again. Use the chips under the search box to switch between All, Unread, Open, Closed and Mine.',
    links: ['chat'],
  },
  {
    id: 'invite_user', question: 'How do I add a teammate?',
    keywords: ['invite', 'add user', 'add agent', 'teammate', 'staff', 'new user', 'agent add', 'employee', 'member'],
    answer: 'Go to **Settings → Users** and click **Add User** (or **Copy Invite Link**). Pick a role to control what they can see. Group agents into **Teams** for automatic chat assignment.',
    links: ['users', 'roles', 'teams'],
  },
  {
    id: 'assign_chat', question: 'How do I assign a chat to an agent?',
    keywords: ['assign', 'assignment', 'assign chat', 'agent ko do', 'transfer chat'],
    answer: 'In the chat, click the **assign** icon (person with +) in the header and pick an agent. Use the **Mine**, **Assigned** and **Unassigned** chips to see who owns what.',
    links: ['chat'],
  },
  {
    id: 'import_contacts', question: 'How do I import contacts?',
    keywords: ['import contacts', 'upload contacts', 'csv', 'excel', 'contacts add', 'contact list'],
    answer: 'Go to **Settings → Contacts** and click **Import/Export**. Download the sample CSV, fill phone numbers with country code (e.g. 9198XXXXXXXX), and upload it.',
    links: ['contacts'],
  },
  {
    id: 'api', question: 'How do I use the API?',
    keywords: ['api', 'api key', 'developer', 'integrate', 'integration', 'webhook', 'crm'],
    answer: 'Create a key in **Settings → API keys** and send it in the `X-API-Key` header. To receive events (new messages, status updates) on your server, add a URL in **Settings → Webhooks**.',
    links: ['api_keys', 'webhooks'],
  },
]

/** Words that signal "take me there" rather than "explain" */
const NAV_VERBS = ['open', 'go to', 'goto', 'take me', 'show me', 'navigate', 'where is', 'where can i find', 'kholo', 'khol do', 'le chalo', 'le jao', 'dikhao', 'kaha hai', 'kahan hai', 'jana hai']

/** Small Hinglish → English bridge so both phrasings hit the same keywords */
const SYNONYMS: Record<string, string> = {
  banao: 'create', banana: 'create', banaye: 'create', bnao: 'create', bhejo: 'send', bhejna: 'send', bhejne: 'send',
  paisa: 'money', paise: 'money', rupay: 'money', rupees: 'money', jodo: 'connect', jode: 'connect', jodna: 'connect',
  kaise: 'how', kese: 'how', kyu: 'why', kyun: 'why', band: 'off', chalu: 'on', naya: 'new', nayi: 'new', sabko: 'all',
}

export function normalizeQuery(q: string): string {
  return q
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s]/gu, ' ')
    .split(/\s+/)
    .filter(Boolean)
    .map(w => SYNONYMS[w] ? `${w} ${SYNONYMS[w]}` : w)
    .join(' ')
}

function scoreKeywords(query: string, words: Set<string>, keywords: string[]): number {
  let score = 0
  for (const k of keywords) {
    if (k.includes(' ')) {
      if (query.includes(k)) score += 3 // exact phrase is a strong signal
      else if (k.split(' ').every(w => words.has(w))) score += 2 // same words, any order (common in Hinglish)
    } else if (words.has(k)) {
      score += 1.5
    } else if (k.length >= 5 && [...words].some(w => w.length >= 4 && (w.startsWith(k) || k.startsWith(w)))) {
      score += 0.75 // plurals and partial words
    }
  }
  return score
}

export interface LocalMatch {
  /** Strong match: answer without asking the AI */
  faq?: AssistantFaq
  /** Plausible match: used when the AI is unavailable */
  weakFaq?: AssistantFaq
  destinations: AssistantDestination[]
  /** "open wallet" style request with one clear target */
  navigateTo?: AssistantDestination
  /** Asked to open a page this user has no access to */
  restricted?: AssistantDestination
  confident: boolean
}

export function matchLocally(rawQuery: string, allowed: AssistantDestination[]): LocalMatch {
  const query = normalizeQuery(rawQuery)
  const words = new Set(query.split(' '))
  const allowedIds = new Set(allowed.map(d => d.id))

  const dests = allowed
    .map(d => ({ d, s: scoreKeywords(query, words, d.keywords) + (query.includes(d.title.toLowerCase()) ? 3 : 0) }))
    .filter(x => x.s > 0)
    .sort((a, b) => b.s - a.s)

  const faqs = FAQS
    .map(f => ({ f, s: scoreKeywords(query, words, f.keywords) + (normalizeQuery(f.question) === query ? 10 : 0) }))
    .filter(x => x.s > 0)
    .sort((a, b) => b.s - a.s)

  const wantsNav = NAV_VERBS.some(v => query.includes(v)) || words.size <= 2
  const top = dests[0]
  const clearWinner = top && (!dests[1] || top.s >= dests[1].s + 1)
  const navigateTo = wantsNav && top && top.s >= 1.5 && clearWinner ? top.d : undefined

  const withAllowedLinks = (f: AssistantFaq) => ({ ...f, links: f.links.filter(id => allowedIds.has(id)) })
  const topFaq = faqs[0]
  const faq = topFaq && topFaq.s >= 3 ? withAllowedLinks(topFaq.f) : undefined
  const faqClear = topFaq && (!faqs[1] || topFaq.s > faqs[1].s)
  const weakFaq = !faq && topFaq && faqClear ? withAllowedLinks(topFaq.f) : undefined

  // A clear navigation request for a page outside the user's permissions
  let restricted: AssistantDestination | undefined
  if (wantsNav && !navigateTo) {
    const hidden = DESTINATIONS
      .filter(d => !allowedIds.has(d.id))
      .map(d => ({ d, s: scoreKeywords(query, words, d.keywords) + (query.includes(d.title.toLowerCase()) ? 3 : 0) }))
      .sort((a, b) => b.s - a.s)[0]
    if (hidden && hidden.s >= 1.5 && hidden.s > (top?.s ?? 0)) restricted = hidden.d
  }

  return {
    faq,
    weakFaq,
    navigateTo,
    restricted,
    destinations: dests.slice(0, 3).map(x => x.d),
    confident: !!navigateTo || !!faq,
  }
}

/** Starter questions for the current page, falling back to general ones */
export function suggestionsFor(path: string): string[] {
  const byPage: Array<[string, string[]]> = [
    ['/chat', ['window_24h', 'bot_off_chat', 'close_chat', 'assign_chat']],
    ['/templates', ['create_template', 'template_rejected', 'campaign']],
    ['/campaigns', ['campaign', 'billing', 'import_contacts']],
    ['/wallet', ['recharge', 'billing', 'message_failed']],
    ['/chatbot', ['chatbot_setup', 'bot_off_chat']],
    ['/settings/accounts', ['connect_number', 'message_failed']],
    ['/settings/users', ['invite_user', 'assign_chat']],
    ['/settings', ['connect_number', 'invite_user', 'api']],
  ]
  const ids = byPage.find(([p]) => path.startsWith(p))?.[1] || ['connect_number', 'create_template', 'recharge', 'message_failed']
  return ids.map(id => FAQS.find(f => f.id === id)!.question)
}
