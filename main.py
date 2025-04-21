import os
import logging
from kubernetes import client, config
from kubernetes.client.rest import ApiException
import redis
from aiogram import Bot, Dispatcher, types, F
from aiogram.filters import Command
from aiogram.utils.keyboard import InlineKeyboardBuilder
from aiogram.types import CallbackQuery
from aiogram.enums import ParseMode
import asyncio

# Настройка логгирования
logging.basicConfig(
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", level=logging.INFO
)
logger = logging.getLogger(__name__)

# Инициализация клиента Kubernetes
try:
    config.load_incluster_config()
except config.ConfigException:
    config.load_kube_config()
k8s_client = client.CoreV1Api()

# Инициализация Redis
redis_client = redis.Redis(
    host=os.getenv("REDIS_HOST", "redis-service"),
    port=int(os.getenv("REDIS_PORT", 6379)),
    password=os.getenv("REDIS_PASSWORD", None),
    decode_responses=True,
)

# Конфигурация Telegram
TELEGRAM_BOT_TOKEN = os.getenv("TELEGRAM_BOT_TOKEN")
TELEGRAM_CHAT_ID = os.getenv("TELEGRAM_CHAT_ID")

# Настройки неймспейсов
NAMESPACES_TO_MONITOR = os.getenv("NAMESPACES_TO_MONITOR", "").split(",")
EXCLUDED_NAMESPACES = os.getenv("EXCLUDED_NAMESPACES", "kube-system").split(",")

# Инициализация бота и диспетчера
bot = Bot(token=TELEGRAM_BOT_TOKEN)
dp = Dispatcher()


# Проверка состояния подов (синхронная функция)
def check_pods():
    problematic_pods = []
    try:
        namespaces = (
            NAMESPACES_TO_MONITOR
            if NAMESPACES_TO_MONITOR
            else [
                ns.metadata.name
                for ns in k8s_client.list_namespace().items
                if ns.metadata.name not in EXCLUDED_NAMESPACES
            ]
        )

        for namespace in namespaces:
            if redis_client.exists(f"pause:{namespace}"):
                logger.info(f"Уведомления для {namespace} приостановлены")
                continue

            try:
                pods = k8s_client.list_namespaced_pod(namespace=namespace)
                for pod in pods.items:
                    if pod.status.phase not in ("Running", "Succeeded"):
                        problematic_pods.append(
                            (namespace, pod.metadata.name, pod.status.phase)
                        )
            except ApiException as e:
                logger.error(f"Ошибка при проверке неймспейса {namespace}: {e}")
    except Exception as e:
        logger.error(f"Общая ошибка: {e}")

    return problematic_pods


# Периодическая проверка
async def scheduled_monitoring():
    while True:
        try:
            problematic_pods = await asyncio.to_thread(check_pods)
            if problematic_pods:
                message = "⚠️ **Проблемные поды:**\n"
                for ns, pod, status in problematic_pods:
                    message += f"- `{ns}/{pod}`: `{status}`\n"
                await bot.send_message(
                    chat_id=TELEGRAM_CHAT_ID,
                    text=message,
                    parse_mode=ParseMode.MARKDOWN,
                )
        except Exception as e:
            logger.error(f"Ошибка в scheduled_monitoring: {e}")
        await asyncio.sleep(60)


# Команда /start
@dp.message(Command("start"))
async def cmd_start(message: types.Message):
    await message.answer("🚀 Бот для мониторинга Kubernetes Pods активен!")


# Команда /pause
@dp.message(Command("pause"))
async def cmd_pause(message: types.Message):
    namespaces = (
        NAMESPACES_TO_MONITOR
        if NAMESPACES_TO_MONITOR
        else [
            ns.metadata.name
            for ns in k8s_client.list_namespace().items
            if ns.metadata.name not in EXCLUDED_NAMESPACES
        ]
    )

    builder = InlineKeyboardBuilder()
    for ns in namespaces:
        builder.button(text=ns, callback_data=f"pause_{ns}")
    builder.adjust(1)

    await message.answer(
        "Выберите неймспейс для приостановки уведомлений:",
        reply_markup=builder.as_markup(),
    )


# Команда /resume
@dp.message(Command("resume"))
async def cmd_resume(message: types.Message):
    paused_namespaces = [key.split(":")[1] for key in redis_client.keys("pause:*")]
    if not paused_namespaces:
        await message.answer("Нет приостановленных неймспейсов.")
        return

    builder = InlineKeyboardBuilder()
    for ns in paused_namespaces:
        builder.button(text=ns, callback_data=f"resume_{ns}")
    builder.adjust(1)

    await message.answer(
        "Выберите неймспейс для возобновления уведомлений:",
        reply_markup=builder.as_markup(),
    )


# Обработка callback кнопок
@dp.callback_query(F.data.startswith("pause_"))
async def pause_callback(callback: CallbackQuery):
    namespace = callback.data.split("_")[1]
    redis_client.setex(f"pause:{namespace}", 3600, "true")
    await callback.message.edit_text(
        f"🔇 Уведомления для `{namespace}` приостановлены на 1 час.",
        parse_mode=ParseMode.MARKDOWN,
    )


@dp.callback_query(F.data.startswith("resume_"))
async def resume_callback(callback: CallbackQuery):
    namespace = callback.data.split("_")[1]
    redis_client.delete(f"pause:{namespace}")
    await callback.message.edit_text(
        f"🔔 Уведомления для `{namespace}` возобновлены.", parse_mode=ParseMode.MARKDOWN
    )


async def on_startup(dispatcher):
    asyncio.create_task(scheduled_monitoring())


if __name__ == "__main__":
    dp.startup.register(on_startup)
    dp.run_polling(bot)
