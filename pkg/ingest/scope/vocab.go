package scope

// Reference scope vocabulary, copied faithfully from banhmi's config-schema
// seed (deploy/seed/scope_term.csv for VN, deploy/seed/scope_term_my.csv for
// MY — the same rows `cmd/seed` loads into banhmi's config.scope_term table).
// banhmi loads these from config so operators can tune without redeploying;
// mise M1a embeds the reference set as Go data and rebuilds a Matcher per
// Discover run — a config-schema loader can replace this later without
// touching the Matcher API. Terms are grouped by class; the trailing comment
// on each group of lines is the seed's theme column, kept as provenance.

// vnStrong is the VN strong class: specific enough to put a document in scope
// for any issuer, matched on số ký hiệu + title + abstract.
var vnStrong = []string{
	"an toàn thông tin", "an toàn hệ thống thông tin", "an toàn thông tin mạng",
	"an ninh mạng", "cấp độ an toàn", "an toàn, bảo mật", "ứng cứu sự cố",
	"mật mã dân sự", // security
	"dữ liệu cá nhân", "bảo vệ dữ liệu cá nhân", "chủ thể dữ liệu",
	"chuyển dữ liệu xuyên biên giới", "cơ sở dữ liệu quốc gia", "trung tâm dữ liệu",
	"dữ liệu cốt lõi", "dữ liệu quan trọng", "định danh điện tử", "xác thực điện tử",
	"sinh trắc học", "căn cước điện tử", "ekyc", "thông tin tín dụng", // data
	"giao dịch điện tử", "chữ ký điện tử", "chứng thư số", "dịch vụ tin cậy",
	"thông điệp dữ liệu", "hợp đồng điện tử", // etransaction
	"điện toán đám mây", // cloud
	"trung gian thanh toán", "ví điện tử", "thanh toán không dùng tiền mặt",
	"thanh toán điện tử", "thanh toán trực tuyến", "thanh toán thẻ",
	"bù trừ điện tử", "chuyển mạch tài chính", "tiền điện tử",
	"thanh toán điện tử liên ngân hàng", "thanh toán QR", "cổng thanh toán",
	"tiền di động", "mobile money", // payments
	"giao diện lập trình ứng dụng mở", "open api", // openbanking
	"ngân hàng điện tử", "ngân hàng trực tuyến", "ngân hàng số",
	"internet banking", "mobile banking", "online banking",
	"ngân hàng đại lý", // digitalbanking
	"thẻ ngân hàng",    // card
	"công nghiệp công nghệ số", "trí tuệ nhân tạo", "hệ thống trí tuệ nhân tạo",
	"hệ thống công nghệ thông tin",                                         // tech
	"rủi ro công nghệ thông tin", "thuê ngoài dịch vụ công nghệ thông tin", // oprisk
	"công nghệ tài chính", "fintech", "cho vay ngang hàng",
	"cơ chế thử nghiệm có kiểm soát", "tài sản số", "tài sản mã hóa", // fintech
}

// vnStrongTitle is the VN strong_title class: in scope for any issuer, matched
// on số ký hiệu + title only (body occurrences are boilerplate-dominated).
var vnStrongTitle = []string{
	"chữ ký số", "chứng thực chữ ký", // etransaction
	"tài khoản thanh toán", // payments
}

// vnWeak is the VN weak class: common technology words that count only with a
// banking signal, matched on số ký hiệu + title only.
var vnWeak = []string{
	"công nghệ thông tin", "công nghệ số", "chuyển đổi số", "hệ thống thông tin",
	"dữ liệu lớn", "chuỗi khối", // tech
	"dịch vụ trực tuyến",                                            // digitalbanking
	"cơ sở dữ liệu", "thông tin khách hàng", "nhận biết khách hàng", // data
	"thuê ngoài", "rủi ro hoạt động", "kiểm soát nội bộ", "kinh doanh liên tục",
	"dự phòng thảm họa", "sao lưu dữ liệu", // oprisk
	"rửa tiền",                                             // aml
	"mã hóa", "tấn công mạng", "lỗ hổng bảo mật", "mã độc", // security
}

// vnSignals is the VN banking-signal class that unlocks weak terms. Match also
// hard-codes an "nhnn" số-ký-hiệu check, so NHNN circulars always carry a signal.
var vnSignals = []string{
	"ngân hàng", "tổ chức tín dụng", "tín dụng", // banking
}

// NewVN returns a Matcher loaded with the embedded VN reference vocabulary
// (banking terms + the NHNN signal set), for the vn-reg corpus.
func NewVN() *Matcher {
	return New(vnStrong, vnStrongTitle, vnWeak, vnSignals)
}

// myStrong is the MY strong class: English finance/bank/payment/data/cyber
// terms specific enough to put a document in scope for any issuer.
var myStrong = []string{
	"cyber security", "cybersecurity", "computer crimes", "information security",
	"network security", "critical information infrastructure", "unauthorised access", // security
	"personal data protection", "data protection", "personal data", "data subject",
	"cross-border data", "credit reporting", "data breach", // data
	"electronic commerce", "electronic transaction", "electronic transactions",
	"electronic signature", "electronic message",
	"electronic know-your-customer", "e-kyc", "ekyc", // etransaction
	"communications and multimedia", "technology risk", "technology risk management",
	"risk management in technology", "rmit", "technology service provider",
	"information and communications technology", // tech
	"electronic money", "e-money", "electronic payment", "payment system",
	"payment systems", "payment instrument", "payment service", "payment services",
	"payment gateway", "e-wallet", // payments
	"digital bank", "digital banking", "internet banking", "mobile banking",
	"online banking", "agent banking", // digitalbanking
	"open finance", "open banking", "open api", "open application programming interface", // openbanking
	"financial services", "islamic financial services", "central bank of malaysia",
	"money services business", "development financial institutions", "banking business", // banking
	"anti-money laundering", "money laundering", "terrorism financing",
	"proceeds of unlawful activities",                         // aml
	"outsourcing", "it outsourcing", "technology outsourcing", // oprisk
	"fintech", "financial technology", "regulatory sandbox", "sandbox", // fintech
	"cloud computing", "cloud services", // cloud
}

// myStrongTitle is the MY strong_title class (title-only strong terms).
var myStrongTitle = []string{
	"digital signature", "certification authority", // etransaction
}

// myWeak is the MY weak class: generic technology words that count only with a
// banking signal.
var myWeak = []string{
	"technology", "information technology", "digital", "electronic",
	"big data", "blockchain", "distributed ledger", // tech
	"data", // data
	"outsourcing", "operational risk", "business continuity",
	"disaster recovery", "data centre", // oprisk
	"cloud",                                                 // cloud
	"financial technology", "fintech", "regulatory sandbox", // fintech
	"penetration testing", "encryption", "malware", "vulnerability", // security
	"digital asset", // fintech
}

// mySignals is the MY banking-signal class that unlocks weak terms.
var mySignals = []string{
	"bank negara", "bnm", "licensed bank", "financial institution", "islamic bank",
	"takaful", "authorised business", "prescribed institution", "banking and financial", // banking
	"approved issuer", // payments
}

// NewMY returns a Matcher loaded with the embedded MY reference vocabulary
// (English finance/bank/payment/data/cyber terms), for the my-reg corpus.
func NewMY() *Matcher {
	return New(myStrong, myStrongTitle, myWeak, mySignals)
}

// For returns the reference Matcher for a corpus jurisdiction ("vn" or "my").
// An unknown jurisdiction returns an empty Matcher — callers must fail open on
// Matcher.Empty() rather than dropping the whole corpus.
func For(jurisdiction string) *Matcher {
	switch jurisdiction {
	case "vn":
		return NewVN()
	case "my":
		return NewMY()
	default:
		return New(nil, nil, nil, nil)
	}
}
