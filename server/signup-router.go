package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type SignupRouter struct {
}

type SignupRequest struct {
	HoneypotFirstname string `json:"firstname"`
	HoneypotLastname  string `json:"lastname"`
	Email             string `json:"email" validate:"required,email"`
	Organization      string `json:"organization" validate:"required"`
	Domain            string `json:"domain" validate:"required"`
	Firstname         string `json:"contactFirstname" validate:"required"`
	Lastname          string `json:"contactLastname" validate:"required"`
	Password          string `json:"password" validate:"required,min=8"`
	Country           string `json:"country" validate:"required,len=2"`
	Language          string `json:"language" validate:"required,len=2"`
	AcceptTerms       bool   `json:"acceptTerms" validate:"required"`
}

func (router *SignupRouter) setupRoutes(s *mux.Router) {
	s.HandleFunc("/confirm/{id}", router.confirm).Methods("POST")
	s.HandleFunc("/", router.signup).Methods("POST")
}

func (router *SignupRouter) signup(w http.ResponseWriter, r *http.Request) {
	var m SignupRequest
	if UnmarshalValidateBody(r, &m) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if m.HoneypotFirstname != "" || m.HoneypotLastname != "" {
		// Honeypot, act as if everything was fine
		w.WriteHeader(http.StatusNoContent)
		return
	}
	domain := strings.ToLower(m.Domain) + ".on.seatsurfing.de"
	if !router.isDomainAvailable(domain) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if !router.isEmailAvailable(m.Email) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if !router.isValidCountryCode(m.Country) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !router.isValidLanguageCode(m.Language) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	signup := &Signup{
		Date:         time.Now(),
		Email:        m.Email,
		Password:     GetUserRepository().GetHashedPassword(m.Password),
		Firstname:    m.Firstname,
		Lastname:     m.Lastname,
		Organization: m.Organization,
		Country:      m.Country,
		Language:     m.Language,
		Domain:       domain,
	}
	if err := GetSignupRepository().Create(signup); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := router.sendDoubleOptInMail(signup, router.getLanguage(signup.Language)); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (router *SignupRouter) confirm(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetSignupRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	if !router.isDomainAvailable(e.Domain) {
		GetSignupRepository().Delete(e)
		w.WriteHeader(http.StatusConflict)
		return
	}
	org := &Organization{
		Name:             e.Organization,
		ContactFirstname: e.Firstname,
		ContactLastname:  e.Lastname,
		ContactEmail:     e.Email,
		Language:         e.Language,
		Country:          e.Country,
	}
	if err := GetOrganizationRepository().Create(org); err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	if err := GetOrganizationRepository().AddDomain(org, e.Domain, true); err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	user := &User{
		Email:          "admin@" + e.Domain,
		HashedPassword: NullString(e.Password),
		OrganizationID: org.ID,
		OrgAdmin:       true,
		SuperAdmin:     false,
	}
	if err := GetUserRepository().Create(user); err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	router.sendConfirmMail(e, router.getLanguage(e.Language))
	GetSignupRepository().Delete(e)
	w.WriteHeader(http.StatusNoContent)
}

func (router *SignupRouter) sendDoubleOptInMail(signup *Signup, language string) error {
	vars := map[string]string{
		"recipientName":  signup.Firstname + " " + signup.Lastname,
		"recipientEmail": signup.Email,
		"confirmID":      signup.ID,
	}
	return sendEmail(signup.Email, "info@seatsurfing.de", EmailTemplateSignup, language, vars)
}

func (router *SignupRouter) sendConfirmMail(signup *Signup, language string) error {
	vars := map[string]string{
		"recipientName":  signup.Firstname + " " + signup.Lastname,
		"recipientEmail": signup.Email,
		"username":       "admin@" + signup.Domain,
	}
	return sendEmail(signup.Email, "info@seatsurfing.de", EmailTemplateConfirm, language, vars)
}

func (router *SignupRouter) getLanguage(language string) string {
	lng := strings.ToLower(language)
	switch lng {
	case "de":
		return lng
	default:
		return "en"
	}
}

func (router *SignupRouter) isValidCountryCode(isoCountryCode string) bool {
	validCountryCodes := []string{"BE", "BG", "DK", "DE", "EE", "FJ", "FR", "GR", "IE", "IT", "HR", "LV", "LT", "LU", "MT", "NL", "AT", "PL", "PT", "RO", "SE", "SK", "SI", "ES", "CZ", "HU", "CY"}
	cc := strings.ToUpper(isoCountryCode)
	for _, s := range validCountryCodes {
		if cc == s {
			return true
		}
	}
	return false
}

func (router *SignupRouter) isValidLanguageCode(isoLanguageCode string) bool {
	validLanguageCodes := []string{"de"}
	lc := strings.ToLower(isoLanguageCode)
	for _, s := range validLanguageCodes {
		if lc == s {
			return true
		}
	}
	return false
}

func (router *SignupRouter) isEmailAvailable(email string) bool {
	org, err := GetOrganizationRepository().GetByEmail(email)
	if (err == nil) && (org != nil) {
		return false
	}
	signup, err := GetSignupRepository().GetByEmail(email)
	if (err == nil) && (signup != nil) {
		return false
	}
	return true
}

func (router *SignupRouter) isDomainAvailable(domain string) bool {
	org, err := GetOrganizationRepository().GetOneByDomain(domain)
	if (err == nil) && (org != nil) {
		return false
	}
	return true
}
