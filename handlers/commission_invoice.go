package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"discord-bot/config"
	"discord-bot/lang"
	"discord-bot/payments"
	"discord-bot/storage"

	"github.com/bwmarrin/discordgo"
)

// stripeCurrencies is the full list of ISO 4217 currency codes supported by Stripe,
// paired with a human-readable label for the autocomplete menu.
var stripeCurrencies = [][2]string{
	{"USD", "USD — US Dollar"},
	{"EUR", "EUR — Euro"},
	{"GBP", "GBP — British Pound"},
	{"CAD", "CAD — Canadian Dollar"},
	{"AUD", "AUD — Australian Dollar"},
	{"CHF", "CHF — Swiss Franc"},
	{"JPY", "JPY — Japanese Yen"},
	{"NZD", "NZD — New Zealand Dollar"},
	{"SEK", "SEK — Swedish Krona"},
	{"NOK", "NOK — Norwegian Krone"},
	{"DKK", "DKK — Danish Krone"},
	{"HKD", "HKD — Hong Kong Dollar"},
	{"SGD", "SGD — Singapore Dollar"},
	{"MXN", "MXN — Mexican Peso"},
	{"BRL", "BRL — Brazilian Real"},
	{"INR", "INR — Indian Rupee"},
	{"PLN", "PLN — Polish Zloty"},
	{"CZK", "CZK — Czech Koruna"},
	{"HUF", "HUF — Hungarian Forint"},
	{"RON", "RON — Romanian Leu"},
	{"BGN", "BGN — Bulgarian Lev"},
	{"HRK", "HRK — Croatian Kuna"},
	{"ISK", "ISK — Icelandic Krona"},
	{"TRY", "TRY — Turkish Lira"},
	{"RUB", "RUB — Russian Ruble"},
	{"ZAR", "ZAR — South African Rand"},
	{"AED", "AED — UAE Dirham"},
	{"SAR", "SAR — Saudi Riyal"},
	{"QAR", "QAR — Qatari Riyal"},
	{"KWD", "KWD — Kuwaiti Dinar"},
	{"BHD", "BHD — Bahraini Dinar"},
	{"OMR", "OMR — Omani Rial"},
	{"ILS", "ILS — Israeli Shekel"},
	{"EGP", "EGP — Egyptian Pound"},
	{"MAD", "MAD — Moroccan Dirham"},
	{"KES", "KES — Kenyan Shilling"},
	{"NGN", "NGN — Nigerian Naira"},
	{"GHS", "GHS — Ghanaian Cedi"},
	{"TZS", "TZS — Tanzanian Shilling"},
	{"UGX", "UGX — Ugandan Shilling"},
	{"KRW", "KRW — South Korean Won"},
	{"CNY", "CNY — Chinese Yuan"},
	{"TWD", "TWD — Taiwan Dollar"},
	{"THB", "THB — Thai Baht"},
	{"MYR", "MYR — Malaysian Ringgit"},
	{"IDR", "IDR — Indonesian Rupiah"},
	{"PHP", "PHP — Philippine Peso"},
	{"VND", "VND — Vietnamese Dong"},
	{"PKR", "PKR — Pakistani Rupee"},
	{"BDT", "BDT — Bangladeshi Taka"},
	{"LKR", "LKR — Sri Lankan Rupee"},
	{"NPR", "NPR — Nepalese Rupee"},
	{"CLP", "CLP — Chilean Peso"},
	{"COP", "COP — Colombian Peso"},
	{"PEN", "PEN — Peruvian Sol"},
	{"ARS", "ARS — Argentine Peso"},
	{"UYU", "UYU — Uruguayan Peso"},
	{"BOB", "BOB — Bolivian Boliviano"},
	{"PYG", "PYG — Paraguayan Guarani"},
	{"GTQ", "GTQ — Guatemalan Quetzal"},
	{"CRC", "CRC — Costa Rican Colon"},
	{"HNL", "HNL — Honduran Lempira"},
	{"NIO", "NIO — Nicaraguan Córdoba"},
	{"DOP", "DOP — Dominican Peso"},
	{"JMD", "JMD — Jamaican Dollar"},
	{"TTD", "TTD — Trinidad Dollar"},
	{"BBD", "BBD — Barbadian Dollar"},
	{"XCD", "XCD — East Caribbean Dollar"},
	{"AWG", "AWG — Aruban Florin"},
	{"ANG", "ANG — Netherlands Antillean Guilder"},
	{"BMD", "BMD — Bermudian Dollar"},
	{"KYD", "KYD — Cayman Islands Dollar"},
	{"FJD", "FJD — Fijian Dollar"},
	{"PGK", "PGK — Papua New Guinean Kina"},
	{"WST", "WST — Samoan Tala"},
	{"TOP", "TOP — Tongan Pa'anga"},
	{"SBD", "SBD — Solomon Islands Dollar"},
	{"VUV", "VUV — Vanuatu Vatu"},
	{"XPF", "XPF — CFP Franc"},
	{"XOF", "XOF — West African CFA Franc"},
	{"XAF", "XAF — Central African CFA Franc"},
	{"GNF", "GNF — Guinean Franc"},
	{"MGA", "MGA — Malagasy Ariary"},
	{"MZN", "MZN — Mozambican Metical"},
	{"ZMW", "ZMW — Zambian Kwacha"},
	{"MWK", "MWK — Malawian Kwacha"},
	{"ETB", "ETB — Ethiopian Birr"},
	{"RWF", "RWF — Rwandan Franc"},
	{"BIF", "BIF — Burundian Franc"},
	{"DJF", "DJF — Djiboutian Franc"},
	{"KMF", "KMF — Comorian Franc"},
	{"MRU", "MRU — Mauritanian Ouguiya"},
	{"SCR", "SCR — Seychellois Rupee"},
	{"MUR", "MUR — Mauritian Rupee"},
	{"MVR", "MVR — Maldivian Rufiyaa"},
	{"BTN", "BTN — Bhutanese Ngultrum"},
	{"MMK", "MMK — Myanmar Kyat"},
	{"KHR", "KHR — Cambodian Riel"},
	{"LAK", "LAK — Lao Kip"},
	{"MNT", "MNT — Mongolian Tugrik"},
	{"AMD", "AMD — Armenian Dram"},
	{"GEL", "GEL — Georgian Lari"},
	{"AZN", "AZN — Azerbaijani Manat"},
	{"KZT", "KZT — Kazakhstani Tenge"},
	{"UZS", "UZS — Uzbekistani Som"},
	{"TJS", "TJS — Tajikistani Somoni"},
	{"KGS", "KGS — Kyrgyzstani Som"},
	{"TMT", "TMT — Turkmenistani Manat"},
	{"AFN", "AFN — Afghan Afghani"},
	{"IRR", "IRR — Iranian Rial"},
	{"IQD", "IQD — Iraqi Dinar"},
	{"LBP", "LBP — Lebanese Pound"},
	{"SYP", "SYP — Syrian Pound"},
	{"JOD", "JOD — Jordanian Dinar"},
	{"YER", "YER — Yemeni Rial"},
	{"SDG", "SDG — Sudanese Pound"},
	{"LYD", "LYD — Libyan Dinar"},
	{"TND", "TND — Tunisian Dinar"},
	{"DZD", "DZD — Algerian Dinar"},
	{"MKD", "MKD — Macedonian Denar"},
	{"ALL", "ALL — Albanian Lek"},
	{"BAM", "BAM — Bosnia-Herzegovina Convertible Mark"},
	{"RSD", "RSD — Serbian Dinar"},
	{"MDL", "MDL — Moldovan Leu"},
	{"UAH", "UAH — Ukrainian Hryvnia"},
	{"BYN", "BYN — Belarusian Ruble"},
	{"GBP", "GBP — British Pound"},
}

func handleInvoiceCurrencyAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Find the focused currency option inside the "create" subcommand
	var query string
	for _, opt := range i.ApplicationCommandData().Options {
		if opt.Name == "create" {
			for _, sub := range opt.Options {
				if sub.Name == "currency" && sub.Focused {
					query = strings.ToUpper(strings.TrimSpace(sub.StringValue()))
				}
			}
		}
	}

	def := defaultInvoiceCurrency()
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, 25)

	// Always show the configured default first when the field is empty
	if query == "" {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  fmt.Sprintf("%s (your default)", def),
			Value: def,
		})
	}

	seen := map[string]bool{}
	for _, pair := range stripeCurrencies {
		code, label := pair[0], pair[1]
		if seen[code] {
			continue
		}
		if query != "" && !strings.HasPrefix(code, query) && !strings.Contains(strings.ToUpper(label), query) {
			continue
		}
		if code == def && query == "" {
			seen[code] = true
			continue // already added as the default entry above
		}
		seen[code] = true
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  label,
			Value: code,
		})
		if len(choices) >= 25 {
			break
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
}

func handleInvoiceCreate(s *discordgo.Session, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	om := subOptMap(opts)
	cfg := storage.Cfg
	gs := storage.GetGuild(i.GuildID)

	client := om["client"].UserValue(s)
	amount := om["amount"].FloatValue()
	description := om["description"].StringValue()
	currency := strings.ToUpper(optStr(om, "currency", defaultInvoiceCurrency()))
	note := optStr(om, "note", "")

	hasGateway := payments.Svc != nil
	paypalEmail := config.EffectiveCommissionPayPalEmail(cfg, gs)
	paypalMe := config.EffectiveCommissionPayPalMe(cfg, gs)
	if !hasGateway && paypalEmail == "" && paypalMe == "" {
		respond(s, i, "❌ No payment method configured. Set up a gateway via `/commission setup` or the `payment` block in `config.json`.", true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	gs.Lock()
	gs.CommissionsRuntime.InvoiceCounter++
	invNum := gs.CommissionsRuntime.InvoiceCounter
	gs.Unlock()

	inv := config.CommissionInvoice{
		Number:      invNum,
		ChannelID:   i.ChannelID,
		GuildID:     i.GuildID,
		ClientID:    client.ID,
		CreatedBy:   i.Member.User.ID,
		Amount:      amount,
		Currency:    currency,
		Description: description,
		Note:        note,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	gatewayButtons, gatewayErrs := buildInvoiceButtons(&inv, paypalMe, amount, currency)
	gatewayErrs = append(gatewayErrs, cryptoPaymentsForInvoice(&inv)...)

	gs.Lock()
	gs.CommissionsRuntime.Invoices = append(gs.CommissionsRuntime.Invoices, inv)
	gs.Unlock()
	_ = gs.Save()

	fields := []*discordgo.MessageEmbedField{
		{Name: lang.T("invoice_field_number"), Value: fmt.Sprintf("`INV-%04d`", invNum), Inline: true},
		{Name: lang.T("invoice_field_client"), Value: fmt.Sprintf("<@%s>", client.ID), Inline: true},
		{Name: lang.T("invoice_field_amount"), Value: invoiceAmountValue(cfg, amount, currency), Inline: true},
		{Name: lang.T("invoice_field_service"), Value: description, Inline: false},
	}
	if len(gatewayButtons) == 0 && paypalEmail != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: lang.T("invoice_field_paypal_to"), Value: fmt.Sprintf("`%s`", paypalEmail), Inline: true,
		})
	}
	if note != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: lang.T("invoice_field_note"), Value: note, Inline: false})
	}

	embed := &discordgo.MessageEmbed{
		Title:       lang.T("invoice_embed_title", "number", fmt.Sprintf("INV-%04d", invNum)),
		Description: lang.T("invoice_embed_desc", "user", client.ID),
		Color:       0xF0A500,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Created by %s • %s", i.Member.User.Username, time.Now().Format("Jan 2, 2006"))},
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if len(inv.CoinbaseCryptoPayments) > 0 {
		gatewayButtons = append(gatewayButtons, cryptoPayButton(invNum))
	}
	send := buildInvoiceSend(fmt.Sprintf("<@%s>", client.ID), embed, gatewayButtons)
	if _, err := s.ChannelMessageSendComplex(i.ChannelID, send); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Failed to post invoice: %s", err.Error()),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	confirmMsg := fmt.Sprintf("✅ Invoice `INV-%04d` posted for <@%s> — **%.2f %s**.", invNum, client.ID, amount, currency)
	if len(gatewayErrs) > 0 {
		var errLines []string
		for _, e := range gatewayErrs {
			errLines = append(errLines, "• "+e.Error())
		}
		confirmMsg += "\n\n⚠️ Some payment gateways failed:\n" + strings.Join(errLines, "\n")
		if len(gatewayButtons) > 0 {
			confirmMsg += "\nThe invoice was posted with the remaining available payment method(s)."
		} else if paypalEmail != "" {
			confirmMsg += "\nThe invoice was posted with the PayPal email as fallback — no payment button is available."
		} else {
			confirmMsg += "\nNo payment methods are available on this invoice."
		}
	}
	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: confirmMsg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

func handleInvoiceList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	invs := gs.CommissionsRuntime.Invoices
	gs.Unlock()

	if len(invs) == 0 {
		respond(s, i, "📭 No invoices on record.", true)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Invoices** (%d total):\n\n", len(invs)))
	for _, inv := range invs {
		paid := "unpaid"
		if inv.Paid {
			paid = "✅ paid"
		}
		sb.WriteString(fmt.Sprintf("`INV-%04d` — <@%s> — **%.2f %s** — %s\n", inv.Number, inv.ClientID, inv.Amount, inv.Currency, paid))
	}
	respond(s, i, sb.String(), true)
}

func handleCommissionInvoiceButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	channelID := strings.TrimPrefix(i.MessageComponentData().CustomID, "commission_invoice_btn:")

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, ok := gs.CommissionsRuntime.OpenCommissions[channelID]
	gs.Unlock()
	if !ok {
		respond(s, i, "❌ Could not find the commission data for this channel.", true)
		return
	}

	descDefault := fmt.Sprintf("%s — %s", ct.ServiceName, ct.Details)
	if len(descDefault) > 100 {
		descDefault = descDefault[:100]
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "commission_invoice_modal:" + channelID,
			Title:    fmt.Sprintf("Issue Invoice — Commission #%04d", ct.Number),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "amount",
						Label:       "Amount to charge",
						Style:       discordgo.TextInputShort,
						Required:    true,
						Placeholder: "e.g. 50.00",
						MaxLength:   20,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "currency",
						Label:       fmt.Sprintf("Currency (default: %s)", defaultInvoiceCurrency()),
						Style:       discordgo.TextInputShort,
						Required:    false,
						Placeholder: defaultInvoiceCurrency(),
						MaxLength:   5,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:  "description",
						Label:     "Invoice description (pre-filled)",
						Style:     discordgo.TextInputParagraph,
						Required:  true,
						Value:     descDefault,
						MaxLength: 500,
					},
				}},
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.TextInput{
						CustomID:    "note",
						Label:       "Additional note (optional)",
						Style:       discordgo.TextInputShort,
						Required:    false,
						Placeholder: "e.g. Due in 7 days",
						MaxLength:   200,
					},
				}},
			},
		},
	})
}

func handleCommissionInvoiceModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ModalSubmitData()
	channelID := strings.TrimPrefix(data.CustomID, "commission_invoice_modal:")

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	ct, ok := gs.CommissionsRuntime.OpenCommissions[channelID]
	gs.Unlock()
	if !ok {
		respond(s, i, "❌ Could not find the commission data. The ticket may have been closed.", true)
		return
	}

	modalFields := modalTextValues(data.Components)
	amountStr := strings.TrimSpace(modalFields["amount"])
	currency := strings.TrimSpace(modalFields["currency"])
	description := strings.TrimSpace(modalFields["description"])
	note := strings.TrimSpace(modalFields["note"])

	amountStr = strings.ReplaceAll(amountStr, ",", ".")
	amount, parseErr := strconv.ParseFloat(amountStr, 64)
	if parseErr != nil || amount <= 0 {
		respond(s, i, "❌ Invalid amount. Please enter a number like `50.00` or `50,00`.", true)
		return
	}
	if currency == "" {
		currency = defaultInvoiceCurrency()
	}
	currency = strings.ToUpper(currency)
	if description == "" {
		description = ct.ServiceName
	}

	cfg := storage.Cfg
	paypalEmail := config.EffectiveCommissionPayPalEmail(cfg, gs)
	paypalMe := config.EffectiveCommissionPayPalMe(cfg, gs)
	if payments.Svc == nil && paypalEmail == "" && paypalMe == "" {
		respond(s, i, "❌ No payment method configured.", true)
		return
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	gs.Lock()
	gs.CommissionsRuntime.InvoiceCounter++
	invNum := gs.CommissionsRuntime.InvoiceCounter
	gs.Unlock()

	inv := config.CommissionInvoice{
		Number:      invNum,
		ChannelID:   channelID,
		GuildID:     i.GuildID,
		ClientID:    ct.UserID,
		CreatedBy:   i.Member.User.ID,
		Amount:      amount,
		Currency:    currency,
		Description: description,
		Note:        note,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	gatewayButtons, gatewayErrs := buildInvoiceButtons(&inv, paypalMe, amount, currency)
	gatewayErrs = append(gatewayErrs, cryptoPaymentsForInvoice(&inv)...)

	gs.Lock()
	gs.CommissionsRuntime.Invoices = append(gs.CommissionsRuntime.Invoices, inv)
	gs.Unlock()
	_ = gs.Save()

	fields := []*discordgo.MessageEmbedField{
		{Name: lang.T("invoice_field_number"), Value: fmt.Sprintf("`INV-%04d`", invNum), Inline: true},
		{Name: lang.T("invoice_field_client"), Value: fmt.Sprintf("<@%s>", ct.UserID), Inline: true},
		{Name: lang.T("invoice_field_amount"), Value: invoiceAmountValue(cfg, amount, currency), Inline: true},
		{Name: lang.T("invoice_field_service"), Value: ct.ServiceName, Inline: true},
		{Name: lang.T("invoice_field_description"), Value: description, Inline: false},
	}
	if len(gatewayButtons) == 0 && paypalEmail != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: lang.T("invoice_field_paypal_to"), Value: fmt.Sprintf("`%s`", paypalEmail), Inline: true,
		})
	}
	if note != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: lang.T("invoice_field_note"), Value: note, Inline: false})
	}

	embed := &discordgo.MessageEmbed{
		Title:       lang.T("invoice_embed_title", "number", fmt.Sprintf("INV-%04d", invNum)),
		Description: lang.T("invoice_embed_desc", "user", ct.UserID),
		Color:       0xF0A500,
		Fields:      fields,
		Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Issued by %s • %s", i.Member.User.Username, time.Now().Format("Jan 2, 2006"))},
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if len(inv.CoinbaseCryptoPayments) > 0 {
		gatewayButtons = append(gatewayButtons, cryptoPayButton(invNum))
	}
	send := buildInvoiceSend(fmt.Sprintf("<@%s>", ct.UserID), embed, gatewayButtons)
	if _, err := s.ChannelMessageSendComplex(channelID, send); err != nil {
		_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Failed to post invoice: %s", err.Error()),
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return
	}

	confirmMsg := fmt.Sprintf("✅ Invoice `INV-%04d` posted — **%.2f %s** for <@%s>.", invNum, amount, currency, ct.UserID)
	if len(gatewayErrs) > 0 {
		var errLines []string
		for _, e := range gatewayErrs {
			errLines = append(errLines, "• "+e.Error())
		}
		confirmMsg += "\n\n⚠️ Some payment gateways failed:\n" + strings.Join(errLines, "\n")
		if len(gatewayButtons) > 0 {
			confirmMsg += "\nThe invoice was posted with the remaining available payment method(s)."
		} else if paypalEmail != "" {
			confirmMsg += "\nThe invoice was posted with the PayPal email as fallback — no payment button is available."
		} else {
			confirmMsg += "\nNo payment methods are available on this invoice."
		}
	}
	_, _ = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: confirmMsg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}

// cryptoPaymentsForInvoice generates Coinbase CDP receive addresses for the
// invoice (one per configured asset) and stores them on inv. Returns any errors.
func cryptoPaymentsForInvoice(inv *config.CommissionInvoice) []error {
	if payments.Svc == nil {
		return nil
	}
	cps, errs := payments.Svc.CreateCryptoPayments(inv)
	if len(cps) > 0 {
		inv.CoinbaseCryptoPayments = cps
	}
	return errs
}

// representativeFee returns the handling fee shared by the enabled gateways and
// whether they all use the same fee. If they differ, uniform is false.
func representativeFee(cfg *config.Config) (fee float64, uniform bool) {
	var fees []float64
	if cfg.Payment.PayPal.Enabled {
		fees = append(fees, cfg.Payment.PayPal.HandlingFee)
	}
	if cfg.Payment.Stripe.Enabled {
		fees = append(fees, cfg.Payment.Stripe.HandlingFee)
	}
	if cfg.Payment.Coinbase.Enabled {
		fees = append(fees, cfg.Payment.Coinbase.HandlingFee)
	}
	if len(fees) == 0 {
		return 0, true
	}
	f0 := fees[0]
	for _, f := range fees {
		if f != f0 {
			return 0, false
		}
	}
	return f0, true
}

// invoiceAmountValue renders the Amount field, showing base + handling fee = total
// when a uniform fee applies, so the client sees what they'll actually pay.
func invoiceAmountValue(cfg *config.Config, amount float64, currency string) string {
	fee, uniform := representativeFee(cfg)
	if !uniform {
		return lang.T("invoice_amount_varies", "amount", fmt.Sprintf("%.2f", amount), "currency", currency)
	}
	if fee <= 0 {
		return lang.T("invoice_amount_plain", "amount", fmt.Sprintf("%.2f", amount), "currency", currency)
	}
	total := amount * (1 + fee)
	return lang.T("invoice_amount_fee",
		"base", fmt.Sprintf("%.2f", amount),
		"fee", strconv.FormatFloat(fee*100, 'f', -1, 64),
		"total", fmt.Sprintf("%.2f", total),
		"currency", currency,
	)
}

// cryptoPayButton is the "Pay with Crypto" component button shown next to the
// PayPal/Stripe buttons. Crypto has no hosted checkout URL, so clicking it opens
// an ephemeral panel with QR codes and addresses instead.
func cryptoPayButton(invNum int) discordgo.Button {
	return discordgo.Button{
		Label:    lang.T("invoice_crypto_button"),
		Style:    discordgo.SecondaryButton,
		CustomID: fmt.Sprintf("invoice_crypto:%d", invNum),
		Emoji:    &discordgo.ComponentEmoji{Name: "🪙"},
	}
}

// cryptoQRURL builds a QR image URL for a crypto payment. BTC uses a BIP21 URI
// (so wallets prefill the amount); others encode the bare address.
func cryptoQRURL(p config.CoinbaseCryptoPayment) string {
	data := p.Address
	if strings.EqualFold(p.Asset, "BTC") {
		data = "bitcoin:" + p.Address + "?amount=" + p.Amount
	}
	return "https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=" + url.QueryEscape(data)
}

// handleInvoiceCryptoButton shows the crypto payment options for an invoice as an
// ephemeral panel: one embed per asset with a QR code, amount, network, address.
func handleInvoiceCryptoButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	numStr := strings.TrimPrefix(i.MessageComponentData().CustomID, "invoice_crypto:")
	num, _ := strconv.Atoi(numStr)

	gs := storage.GetGuild(i.GuildID)
	gs.Lock()
	var payments []config.CoinbaseCryptoPayment
	for _, inv := range gs.CommissionsRuntime.Invoices {
		if inv.Number == num {
			payments = inv.CoinbaseCryptoPayments
			break
		}
	}
	gs.Unlock()

	if len(payments) == 0 {
		respond(s, i, lang.T("invoice_crypto_none"), true)
		return
	}

	embeds := make([]*discordgo.MessageEmbed, 0, len(payments))
	for _, p := range payments {
		var title, desc string
		if p.Network != "" {
			title = lang.T("invoice_crypto_name_net", "asset", p.Asset, "network", p.Network)
			desc = lang.T("invoice_crypto_value_net", "amount", p.Amount, "asset", p.Asset, "network", p.Network, "address", p.Address)
		} else {
			title = lang.T("invoice_crypto_name", "asset", p.Asset)
			desc = lang.T("invoice_crypto_value", "amount", p.Amount, "asset", p.Asset, "address", p.Address)
		}
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:       title,
			Description: desc,
			Color:       0xF0A500,
			Image:       &discordgo.MessageEmbedImage{URL: cryptoQRURL(p)},
		})
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: lang.T("invoice_crypto_panel_title", "number", fmt.Sprintf("INV-%04d", num)),
			Embeds:  embeds,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// buildInvoiceButtons assembles payment link buttons from the gateway and/or PayPal.me fallback.
// It returns the buttons and any gateway errors so callers can surface failures to staff.
func buildInvoiceButtons(inv *config.CommissionInvoice, paypalMe string, amount float64, currency string) ([]discordgo.MessageComponent, []error) {
	var buttons []discordgo.MessageComponent
	var gatewayErrs []error
	if payments.Svc != nil {
		links, errs := payments.Svc.CreateLinks(inv)
		gatewayErrs = errs
		for _, btn := range links {
			buttons = append(buttons, discordgo.Button{
				Label: btn.Label,
				Style: discordgo.LinkButton,
				URL:   btn.URL,
				Emoji: &discordgo.ComponentEmoji{Name: btn.Emoji},
			})
		}
	}
	if len(buttons) == 0 && paypalMe != "" {
		paypalURL := fmt.Sprintf("https://paypal.me/%s/%.2f%s", paypalMe, amount, currency)
		buttons = append(buttons, discordgo.Button{
			Label: lang.T("invoice_paypalme_button", "amount", fmt.Sprintf("%.2f", amount), "currency", currency),
			Style: discordgo.LinkButton,
			URL:   paypalURL,
			Emoji: &discordgo.ComponentEmoji{Name: "💳"},
		})
	}
	return buttons, gatewayErrs
}

// buildInvoiceSend creates the MessageSend with the embed and button rows (max 5 per row).
func buildInvoiceSend(content string, embed *discordgo.MessageEmbed, buttons []discordgo.MessageComponent) *discordgo.MessageSend {
	send := &discordgo.MessageSend{
		Content: content,
		Embeds:  []*discordgo.MessageEmbed{embed},
	}
	for start := 0; start < len(buttons); start += 5 {
		end := start + 5
		if end > len(buttons) {
			end = len(buttons)
		}
		send.Components = append(send.Components, discordgo.ActionsRow{
			Components: buttons[start:end],
		})
	}
	return send
}
