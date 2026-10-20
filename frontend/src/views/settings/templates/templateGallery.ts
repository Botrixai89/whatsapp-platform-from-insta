/**
 * Ready-made WhatsApp templates businesses can start from. Each one is written to
 * pass Meta review: variables sit inside sentences, marketing messages carry an
 * opt-out line, and every variable has a sample value.
 */

export interface GalleryButton {
  type: 'QUICK_REPLY' | 'URL' | 'PHONE_NUMBER' | 'COPY_CODE'
  text: string
  url?: string
  phone_number?: string
  example?: string
}

export interface GalleryTemplate {
  id: string
  industry: GalleryIndustry
  title: string
  name: string
  category: 'MARKETING' | 'UTILITY'
  header?: string
  body: string
  footer?: string
  buttons: GalleryButton[]
  samples: string[]
}

export type GalleryIndustry = 'ecommerce' | 'food' | 'festive' | 'education' | 'travel' | 'healthcare' | 'realestate' | 'other'

export const galleryIndustries: GalleryIndustry[] = ['ecommerce', 'food', 'festive', 'education', 'travel', 'healthcare', 'realestate', 'other']

const OPT_OUT = 'Reply STOP to unsubscribe'

export const galleryTemplates: GalleryTemplate[] = [
  // E-commerce
  {
    id: 'order_confirmed', industry: 'ecommerce', title: 'Order confirmed', name: 'order_confirmed', category: 'UTILITY',
    header: 'Order confirmed',
    body: 'Hi {{1}}, thank you for shopping with us! Your order #{{2}} of ₹{{3}} is confirmed and will be packed shortly. We will message you as soon as it ships.',
    buttons: [{ type: 'URL', text: 'View order', url: 'https://example.com/orders/{{1}}', example: 'https://example.com/orders/10234' }],
    samples: ['Priya', '10234', '1,499'],
  },
  {
    id: 'order_shipped', industry: 'ecommerce', title: 'Out for delivery', name: 'order_out_for_delivery', category: 'UTILITY',
    header: 'Out for delivery',
    body: 'Hello {{1}}, your order #{{2}} is out for delivery and should reach you by {{3}}. Please keep your phone nearby so our delivery partner can reach you.',
    buttons: [{ type: 'URL', text: 'Track order', url: 'https://example.com/track/{{1}}', example: 'https://example.com/track/10234' }],
    samples: ['Rahul', '10234', '7 PM today'],
  },
  {
    id: 'abandoned_cart', industry: 'ecommerce', title: 'Abandoned cart', name: 'cart_reminder', category: 'MARKETING',
    body: 'Hi {{1}}, you left {{2}} in your cart 🛒 It is still available, but stock is running low. Complete your order now and get free delivery.',
    footer: OPT_OUT,
    buttons: [{ type: 'URL', text: 'Complete order', url: 'https://example.com/cart' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Anjali', 'the blue cotton kurta'],
  },
  {
    id: 'browse_catalog', industry: 'ecommerce', title: 'Browse our catalog', name: 'browse_catalog', category: 'MARKETING',
    header: 'New arrivals are here',
    body: 'Hey {{1}} 👋 Our new collection just dropped! Browse it right here on WhatsApp and we will help you pick the perfect fit.',
    footer: OPT_OUT,
    buttons: [{ type: 'QUICK_REPLY', text: 'Show products' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Neha'],
  },
  {
    id: 'cod_confirm', industry: 'ecommerce', title: 'COD confirmation', name: 'cod_order_confirmation', category: 'UTILITY',
    body: 'Hi {{1}}, you placed a Cash on Delivery order #{{2}} for ₹{{3}}. Please confirm so we can ship it today.',
    buttons: [{ type: 'QUICK_REPLY', text: 'Confirm order' }, { type: 'QUICK_REPLY', text: 'Cancel order' }],
    samples: ['Vikram', '10235', '899'],
  },

  // Food delivery
  {
    id: 'food_delivered', industry: 'food', title: 'Order delivered', name: 'food_order_delivered', category: 'UTILITY',
    body: 'Hey {{1}}, your order from {{2}} has been delivered 🍔 We hope you enjoy your meal! Tell us how it was so we can keep getting better.',
    buttons: [{ type: 'QUICK_REPLY', text: 'Loved it' }, { type: 'QUICK_REPLY', text: 'Could be better' }],
    samples: ['Arjun', 'Spice Garden'],
  },
  {
    id: 'food_offer', industry: 'food', title: 'Weekend offer', name: 'weekend_food_offer', category: 'MARKETING',
    header: 'Weekend special 🍕',
    body: 'Hi {{1}}, craving something delicious? Get {{2}} off on your next order this weekend with code {{3}}. Hurry, the offer ends Sunday midnight!',
    footer: OPT_OUT,
    buttons: [{ type: 'URL', text: 'Order now', url: 'https://example.com/menu' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Sneha', '20%', 'WEEKEND20'],
  },

  // Festive
  {
    id: 'diwali_sale', industry: 'festive', title: 'Diwali sale', name: 'diwali_sale', category: 'MARKETING',
    header: 'Happy Diwali 🪔',
    body: 'Hi {{1}}, celebrate the festival of lights with up to {{2}} off on our entire range! Shop before {{3}} to get your order in time for the festivities.',
    footer: OPT_OUT,
    buttons: [{ type: 'URL', text: 'Shop the sale', url: 'https://example.com/diwali' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Pooja', '50%', '30th October'],
  },
  {
    id: 'festive_greeting', industry: 'festive', title: 'Festival greeting', name: 'festival_greeting', category: 'MARKETING',
    body: 'Dear {{1}}, wishing you and your family a very happy {{2}}! Thank you for being part of our journey. Here is a small gift from us: use code {{3}} on your next order.',
    footer: OPT_OUT,
    buttons: [{ type: 'COPY_CODE', text: 'Copy code', example: 'FESTIVE15' }],
    samples: ['Mr. Sharma', 'Navratri', 'FESTIVE15'],
  },

  // Education
  {
    id: 'admission_inquiry', industry: 'education', title: 'Admission inquiry', name: 'admission_inquiry_followup', category: 'MARKETING',
    header: 'Thank you for your interest',
    body: 'Hi {{1}}, thank you for your interest in {{2}}. Admissions close on {{3}} and seats are filling fast. Our counsellor can help you with fees, eligibility and scholarships.',
    footer: OPT_OUT,
    buttons: [{ type: 'QUICK_REPLY', text: 'Talk to counsellor' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Aditya', 'our B.Tech programme', '15th November'],
  },
  {
    id: 'exam_reminder', industry: 'education', title: 'Exam reminder', name: 'exam_reminder', category: 'UTILITY',
    body: 'Hello {{1}}, this is a reminder that your {{2}} exams begin on {{3}}. Please carry your admit card and reach the centre 30 minutes early. All the best!',
    buttons: [{ type: 'URL', text: 'View timetable', url: 'https://example.com/timetable' }],
    samples: ['Riya', 'mid-term', 'Monday, 20th October'],
  },
  {
    id: 'fee_reminder', industry: 'education', title: 'Fee reminder', name: 'fee_payment_reminder', category: 'UTILITY',
    body: 'Dear parent, the {{1}} fee of ₹{{2}} for {{3}} is due on {{4}}. Please pay on time to avoid a late fee.',
    buttons: [{ type: 'URL', text: 'Pay now', url: 'https://example.com/fees' }],
    samples: ['term 2', '18,500', 'Aarav (Class 6)', '25th October'],
  },

  // Travel
  {
    id: 'travel_offer', industry: 'travel', title: 'Travel sale', name: 'travel_sale', category: 'MARKETING',
    header: 'The big travel sale ✈️',
    body: 'Hi {{1}}, the ultimate travel sale is here! Enjoy up to {{2}} off on flights and hotels with code {{3}}. Hurry, the offer ends on {{4}}.',
    footer: OPT_OUT,
    buttons: [{ type: 'URL', text: 'Book now', url: 'https://example.com/offers' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Karan', '25%', 'WANDER25', '31st October'],
  },
  {
    id: 'booking_confirmed', industry: 'travel', title: 'Booking confirmed', name: 'booking_confirmation', category: 'UTILITY',
    header: 'Booking confirmed',
    body: 'Hi {{1}}, your booking {{2}} is confirmed for {{3}}. We will send your tickets and check-in details a day before you travel. Have a great trip!',
    buttons: [{ type: 'URL', text: 'View booking', url: 'https://example.com/bookings' }],
    samples: ['Meera', 'BKX4821', '12th December'],
  },
  {
    id: 'flight_delay', industry: 'travel', title: 'Flight delayed', name: 'flight_delay_update', category: 'UTILITY',
    body: 'Hi {{1}}, your flight {{2}} is delayed. The new departure time is {{3}}. We are sorry for the inconvenience and will keep you updated.',
    buttons: [{ type: 'QUICK_REPLY', text: 'Talk to support' }],
    samples: ['Rohit', '6E 2134', '9:40 PM'],
  },

  // Healthcare
  {
    id: 'appointment_reminder', industry: 'healthcare', title: 'Appointment reminder', name: 'appointment_reminder', category: 'UTILITY',
    body: 'Hello {{1}}, this is a reminder of your appointment with {{2}} on {{3}}. Please reply to confirm, or reschedule if the time no longer works for you.',
    buttons: [{ type: 'QUICK_REPLY', text: 'Confirm' }, { type: 'QUICK_REPLY', text: 'Reschedule' }],
    samples: ['Sunita', 'Dr. Mehta', 'Friday at 11:30 AM'],
  },
  {
    id: 'report_ready', industry: 'healthcare', title: 'Lab report ready', name: 'lab_report_ready', category: 'UTILITY',
    body: 'Hi {{1}}, your {{2}} report is ready. You can download it securely using the link below. Contact us if you have any questions.',
    buttons: [{ type: 'URL', text: 'Download report', url: 'https://example.com/reports' }],
    samples: ['Amit', 'blood test'],
  },
  {
    id: 'refill_reminder', industry: 'healthcare', title: 'Refill reminder', name: 'medicine_refill_reminder', category: 'UTILITY',
    body: 'Hi {{1}}, your {{2}} will run out in about {{3}} days. Reply Reorder and we will deliver a refill to your doorstep.',
    buttons: [{ type: 'QUICK_REPLY', text: 'Reorder' }, { type: 'QUICK_REPLY', text: 'Not now' }],
    samples: ['Manoj', 'CPAP mask cushions', '5'],
  },

  // Real estate
  {
    id: 'site_visit', industry: 'realestate', title: 'Site visit invite', name: 'site_visit_invite', category: 'MARKETING',
    header: 'Your dream home awaits',
    body: 'Hi {{1}}, thank you for your interest in {{2}}. We would love to show you around. Book a free site visit this weekend and get an exclusive early-bird price.',
    footer: OPT_OUT,
    buttons: [{ type: 'QUICK_REPLY', text: 'Book site visit' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Deepak', 'Green Valley Residency'],
  },
  {
    id: 'visit_confirmed', industry: 'realestate', title: 'Visit confirmed', name: 'site_visit_confirmed', category: 'UTILITY',
    body: 'Hi {{1}}, your site visit to {{2}} is confirmed for {{3}}. Our executive {{4}} will meet you at the site office.',
    buttons: [{ type: 'URL', text: 'Get directions', url: 'https://maps.google.com' }],
    samples: ['Deepak', 'Green Valley Residency', 'Saturday at 4 PM', 'Rakesh'],
  },

  // Others
  {
    id: 'feedback_survey', industry: 'other', title: 'Feedback survey', name: 'feedback_survey', category: 'MARKETING',
    body: 'Hello {{1}}, we hope you liked your recent purchase! Your feedback matters a lot to us. How was your experience?',
    footer: OPT_OUT,
    buttons: [{ type: 'QUICK_REPLY', text: 'Loved it' }, { type: 'QUICK_REPLY', text: 'It was okay' }, { type: 'QUICK_REPLY', text: 'Not happy' }],
    samples: ['Kavita'],
  },
  {
    id: 'we_miss_you', industry: 'other', title: 'We miss you', name: 'win_back_offer', category: 'MARKETING',
    body: 'Hello {{1}}, we have missed you! Come back and enjoy {{2}} off your next purchase with code {{3}}. This offer is valid till {{4}}.',
    footer: OPT_OUT,
    buttons: [{ type: 'URL', text: 'Shop now', url: 'https://example.com' }, { type: 'QUICK_REPLY', text: 'Stop' }],
    samples: ['Sanjay', '20%', 'COMEBACK20', '15th December'],
  },
  {
    id: 'payment_reminder', industry: 'other', title: 'Payment reminder', name: 'payment_due_reminder', category: 'UTILITY',
    body: 'Hi {{1}}, this is a friendly reminder that your payment of ₹{{2}} for invoice {{3}} is due on {{4}}. Please ignore this message if you have already paid.',
    buttons: [{ type: 'URL', text: 'Pay now', url: 'https://example.com/pay' }],
    samples: ['Nikhil', '4,500', 'INV-2291', '20th October'],
  },
  {
    id: 'callback_request', industry: 'other', title: 'Callback request', name: 'callback_request', category: 'UTILITY',
    body: 'Hi {{1}}, we tried calling you about your request {{2}} but could not reach you. When is a good time to call back?',
    buttons: [{ type: 'QUICK_REPLY', text: 'Call me now' }, { type: 'QUICK_REPLY', text: 'Call me later' }],
    samples: ['Farhan', '#45821'],
  },
]

/** Converts a gallery entry into the template editor's form fields. */
export function galleryToDraft(g: GalleryTemplate) {
  return {
    name: g.name,
    display_name: g.title,
    language: 'en',
    category: g.category,
    header_type: g.header ? 'TEXT' : 'NONE',
    header_content: g.header || '',
    body_content: g.body,
    footer_content: g.footer || '',
    buttons: g.buttons.map(b => ({ ...b })),
    sample_values: g.samples.map((value, i) => ({ component: 'body', index: i + 1, value })),
  }
}
