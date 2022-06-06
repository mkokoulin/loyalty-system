package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/KokoulinM/go-musthave-diploma-tpl/cmd/gophermart/config"
	"github.com/KokoulinM/go-musthave-diploma-tpl/internal/auth"
	"github.com/KokoulinM/go-musthave-diploma-tpl/internal/handlers/middlewares"
	"github.com/KokoulinM/go-musthave-diploma-tpl/internal/models"
	"github.com/KokoulinM/go-musthave-diploma-tpl/internal/utils"
)

type Repository interface {
	CreateUser(ctx context.Context, user models.User) (*models.User, error)
	CheckPassword(ctx context.Context, user models.User) (*models.User, error)
	CreateOrder(ctx context.Context, order models.Order) error
	GetOrders(ctx context.Context, userID string) ([]models.ResponseOrderWithAccrual, error)
	GetBalance(ctx context.Context, userID string) (models.UserBalance, error)
	CreateWithdraw(ctx context.Context, withdraw models.Withdraw, userID string) error
}

type Handlers struct {
	repo Repository
	cfg  *config.Config
}

type ErrorWithDB struct {
	Err   error
	Title string
}

func (err *ErrorWithDB) Error() string {
	return fmt.Sprintf("%v", err.Err)
}

func (err *ErrorWithDB) Unwrap() error {
	return err.Err
}

func NewErrorWithDB(err error, title string) error {
	return &ErrorWithDB{
		Err:   err,
		Title: title,
	}
}

func New(repo Repository, cfg *config.Config) *Handlers {
	return &Handlers{
		repo: repo,
		cfg:  cfg,
	}
}

func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	log.Println("Register")
	r.Header.Add("Content-Type", "application/json; charset=utf-8")

	user := models.User{}

	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(body) == 0 {
		http.Error(w, "the body is missing", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(body, &user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newUser, err := h.repo.CreateUser(r.Context(), user)
	var dbErr *ErrorWithDB

	if errors.As(err, &dbErr) && dbErr.Title == "UniqConstraint" {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	if errors.As(err, &dbErr) && dbErr.Title == "UndefinedTable" {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := auth.CreateToken(newUser.ID, h.cfg.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Authorization", "Bearer "+token.AccessToken)

	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	log.Println("Login")
	r.Header.Add("Content-Type", "application/json; charset=utf-8")

	user := models.User{}

	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(body) == 0 {
		http.Error(w, "the body is missing", http.StatusBadRequest)
		return
	}

	err = json.Unmarshal(body, &user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	newUser, err := h.repo.CheckPassword(r.Context(), user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	token, err := auth.CreateToken(newUser.ID, h.cfg.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Authorization", "Bearer "+token.AccessToken)

	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) CreateOrder(w http.ResponseWriter, r *http.Request) {
	log.Println("CreateOrder")
	defer r.Body.Close()

	r.Header.Add("Content-Type", "text/plain")

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(body) == 0 {
		http.Error(w, "the body is missing", http.StatusBadRequest)
		return
	}

	number, err := strconv.Atoi(string(body))

	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	if !utils.ValidLuhnNumber(number) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	userIDCtx := r.Context().Value(middlewares.UserIDCtx).(string)

	order := models.Order{
		UserID: userIDCtx,
		Number: strconv.Itoa(number),
		Status: "NEW",
	}

	err = h.repo.CreateOrder(r.Context(), order)
	if err != nil {
		var dbErr *ErrorWithDB

		if errors.As(err, &dbErr) && dbErr.Title == "OrderAlreadyRegisterByYou" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if errors.As(err, &dbErr) && dbErr.Title == "OrderAlreadyRegister" {
			w.WriteHeader(http.StatusConflict)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *Handlers) GetOrders(w http.ResponseWriter, r *http.Request) {
	log.Println("GetOrders")
	userIDCtx := r.Context().Value(middlewares.UserIDCtx).(string)

	log.Println("GetOrders")

	orders, err := h.repo.GetOrders(r.Context(), userIDCtx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := json.Marshal(orders)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)

	_, err = w.Write(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	log.Println("GetBalance")
	userIDCtx := r.Context().Value(middlewares.UserIDCtx).(string)

	userBalance, err := h.repo.GetBalance(r.Context(), userIDCtx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := json.Marshal(userBalance)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusOK)

	_, err = w.Write(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handlers) CreateWithdraw(w http.ResponseWriter, r *http.Request) {
	log.Println("CreateWithdraw")
	defer r.Body.Close()

	withdraw := models.Withdraw{}

	userIDCtx := r.Context().Value(middlewares.UserIDCtx).(string)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(body, &withdraw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	number, err := strconv.Atoi(string(withdraw.Order))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !utils.ValidLuhnNumber(number) {
		http.Error(w, "", http.StatusUnprocessableEntity)
		return
	}

	err = h.repo.CreateWithdraw(r.Context(), withdraw, userIDCtx)
	if err != nil {
		var dbErr *ErrorWithDB

		if errors.As(err, &dbErr) && dbErr.Title == "NotEnoughBalanceForWithdraw" {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
