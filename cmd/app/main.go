package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var (
	redisClient   *redis.Client
	bot           *tgbotapi.BotAPI
	allowedUsers  = make(map[int64]bool)
	namespaces    []string
	excludedNS    = map[string]bool{"kube-system": true, "kube-public": true}
	pauseDuration = 1 * time.Hour
)

const (
	redisPrefix = "pause:"
)

func main() {
	ctx := context.Background()

	// Инициализация Redis
	initRedis(ctx)

	// Инициализация Telegram бота
	initTelegram()

	// Загрузка конфигурации Kubernetes
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// Загрузка списка неймспейсов
	loadNamespaces(clientset)

	// Настройка информера
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		5*time.Minute,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = "status.phase!=Running,status.phase!=Succeeded"
		}),
	)

	informer := factory.Core().V1().Pods().Informer()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			handlePodEvent(pod)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			newPod := newObj.(*corev1.Pod)
			handlePodEvent(newPod)
		},
	})

	// Запуск информера
	stopCh := make(chan struct{})
	go informer.Run(stopCh)

	// Обработка сигналов
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	close(stopCh)
}

func initRedis(ctx context.Context) {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     getEnv("REDIS_HOST", "redis-service:6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
	})

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
}

func initTelegram() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	// Загрузка ID администраторов
	for _, id := range strings.Split(os.Getenv("ADMIN_USER_IDS"), ",") {
		if id != "" {
			allowedUsers[time.(id)] = true
		}
	}

	// Обработчик команд
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	go func() {
		for update := range updates {
			if update.Message == nil {
				continue
			}

			if !allowedUsers[update.Message.From.ID] {
				sendMessage(update.Message.Chat.ID, "⛔ Access denied")
				continue
			}

			switch update.Message.Command() {
			case "start":
				sendMessage(update.Message.Chat.ID, "🚀 K8s monitoring bot is active!")
			case "pause":
				showNamespaceKeyboard(update.Message.Chat.ID, "pause")
			case "resume":
				showResumeKeyboard(update.Message.Chat.ID)
			}
		}
	}()
}

func handlePodEvent(pod *corev1.Pod) {
	if isExcluded(pod.Namespace) || isPaused(pod.Namespace) {
		return
	}

	if isProblematic(pod) {
		msg := fmt.Sprintf("⚠️ Problematic pod: %s/%s\nPhase: %s",
			pod.Namespace,
			pod.Name,
			pod.Status.Phase)

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && isCriticalReason(cs.State.Waiting.Reason) {
				msg += fmt.Sprintf("\nContainer %s: %s", cs.Name, cs.State.Waiting.Reason)
			}
		}

		sendMessage(parseInt64(os.Getenv("TELEGRAM_CHAT_ID")), msg)
	}
}

func isProblematic(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
		return true
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && isCriticalReason(cs.State.Waiting.Reason) {
			return true
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return true
		}
	}
	return false
}

func showNamespaceKeyboard(chatID int64, action string) {
	msg := tgbotapi.NewMessage(chatID, "Select namespace:")
	keyboard := tgbotapi.NewInlineKeyboardMarkup()

	for _, ns := range namespaces {
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(ns, fmt.Sprintf("%s:%s", action, ns)),
		)
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
	}

	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

func isPaused(namespace string) bool {
	ctx := context.Background()
	val, err := redisClient.Get(ctx, redisPrefix+namespace).Result()
	return err == nil && val == "true"
}

// Вспомогательные функции
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	bot.Send(msg)
}
