<div align="center">

<img src="assets/banner.svg" alt="Notification System" width="100%"/>

[English](README.md) · **فارسی**

</div>

<div dir="rtl" align="right">

## درباره‌ی پروژه

این مخزن راه‌حل من برای **پروژه‌ی رایانش ابری** دانشگاه فردوسی مشهد است. دو میکروسرویس آماده‌ی یک سیستم مدیریت و ارسال
نوتیفیکیشن (سرویس احراز هویت با Go و سرویس اصلی با Spring Boot) در چهار فاز **داکرایز، مستقر، امن، توسعه، مقیاس‌پذیر و
متعادل‌سازی** شده‌اند.

ویژگی این مخزن این است که **همه‌چیز مستند شده است**: هر گام، دلیل هر تصمیم، دستورهای اجراشده، روش راستی‌آزمایی، و
**همه‌ی مشکلاتی که با آن‌ها روبه‌رو شدم و راه حلشان**.

> کد خود دو سرویس درس در این مخزن **نیست**. فقط چیزهایی که خودم نوشته‌ام آمده است: Dockerfileها، فایل‌های Compose و Kubernetes،
> میکروسرویس اضافه، اسکریپت‌ها و مستندات.

## چهار فاز

| فاز | موضوع | خروجی اصلی |
|---|---|---|
| ۱ | داکرایز | Dockerfile چندمرحله‌ای، Compose، شبکه‌ی جدا برای هر سرویس، دیتابیس ماندگار، پورت‌های فقط روی `127.0.0.1` |
| ۲ | Kubernetes | Namespace، ConfigMap، Secret، PV/PVC دستی، Deployment با probe و limit، Service، NetworkPolicy با Cilium |
| ۳ | میکروسرویس شخصی | سرویس Go با `GET /auth/users` (صفحه‌بندی‌شده) و `DELETE /auth/user` (با ID یا ایمیل) در namespace جدا |
| ۴ | مقیاس‌پذیری و مسیریابی | metrics-server، HPA، گیت‌وی NGINX با upstream پویا، کنترل‌پلین با دسترسی‌پذیری بالا |

## معماری

نمودار کامل (Mermaid) در [README انگلیسی](README.md#architecture) است. خلاصه:

- کاربر از **یک NodePort (۳۰۱۰۰)** وارد **NGINX** می‌شود؛ مسیر `/user/*` به auth، مسیر `/auth/*` به user-manager و بقیه به core می‌رود.
- هر سرویس همراه دیتابیس خودش در یک **namespace** جدا است و با NetworkPolicy فقط مسیرهای لازم باز است؛ مثلاً
  user-manager فقط به `auth-db` دسترسی دارد و پورت gRPC (۵۰۰۵۱) فقط برای core باز است.
- `auth`، `core` و `user-manager` با **HPA** بین حداقل و حداکثر replica مقیاس می‌گیرند؛ دیتابیس‌ها و broker تک‌نمونه‌اند.

## مستندات

| سند | محتوا |
|---|---|
| [نمای کلی پروژه](docs/00-project-overview.md) | سیستم چه می‌کند و چه چیزهایی از خواندن سورس فهمیدم |
| [فاز ۱](docs/01-phase1-docker.md) | Dockerfileها خط‌به‌خط، Compose، شبکه‌ها، تست‌ها |
| [فاز ۲](docs/02-phase2-kubernetes.md) | ذخیره‌سازی، ConfigMap/Secret، workloadها، NetworkPolicy |
| [فاز ۳](docs/03-phase3-microservice.md) | قرارداد API، توضیح کد، استقرار و ایزوله‌سازی |
| [فاز ۴](docs/04-phase4-autoscaling-gateway.md) | metrics-server، HPA، NGINX، تست بار، HA |
| [مشکلات و راه‌حل‌ها](docs/TROUBLESHOOTING.md) | ۲۴ مشکل با علت و راه حل |
| [همه‌ی دستورها](docs/COMMANDS.md) | تمام دستورهای استفاده‌شده به ترتیب فاز |

> مستندات به انگلیسی نوشته شده‌اند تا برای همه قابل استفاده باشند.

## اجرای سریع

```bash
# فاز ۱
cp phase1-docker/dockerfiles/auth/{Dockerfile,.dockerignore} ../fum-cloud-notification-auth-2025/
cp phase1-docker/dockerfiles/core/{Dockerfile,.dockerignore} ../fum-cloud-notification-core-2025/
cd phase1-docker && cp .env.example .env     # رمزها را عوض کنید
docker compose up -d --build

# فازهای ۲ تا ۴ (ایمیج‌ها باید روی نودها import شوند)
./tools/load-image.sh auth:phase1 && ./tools/load-image.sh core:phase1
./phase2-kubernetes/apply.sh
./phase3-microservice/apply.sh
./phase4-autoscaling-gateway/apply.sh
```

## نکات مهم

- همه‌ی رمزهای داخل مخزن **نمونه** هستند و باید عوض شوند.
- سرویس `user-manager` احراز هویت ندارد (در صورت پروژه خواسته نشده بود).
- فایل واقعی `haproxy.cfg` باید در [`phase4-autoscaling-gateway/ha`](phase4-autoscaling-gateway/ha/README.md) اضافه شود.

## نویسنده

**علی کاشکی** · DevOps / SRE · [github.com/alikashki](https://github.com/alikashki)

</div>
