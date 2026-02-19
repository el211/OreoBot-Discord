package handlers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"discord-bot/config"
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