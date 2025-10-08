# 👗 AI Stylist — Intelligent Fashion Assistant (in development)

**AI Stylist** is an AI-powered fashion assistant designed to analyze clothing, suggest outfits, and generate detailed metadata from images.  
Currently, the system is in early development, with the main interaction available through the Telegram bot.

---

## 🌐 Live Links

- **Website:** [https://capsuleme.ru/](https://capsuleme.ru/)  
- **Telegram Bot:** [@CapsuleMeBot](https://t.me/CapsuleMeBot)

---

## 🚧 Project Status

At the moment, the website redirects to the Telegram bot for testing and interaction.  
The backend logic and computer vision features are under active development.

---

## 🧩 Planned Architecture

The final version of **AI Stylist** will be built as a **microservice system** with the following components:

- 🧠 **Core Service (Go):**  
  Handles authentication, user sessions, and orchestrates the pipeline via gRPC.

- 🖼️ **Computer Vision Module (Python):**  
  Generates clothing descriptions, extracts color and texture, and processes image embeddings.

- 💬 **gRPC Communication:**  
  Enables fast and secure data exchange between services.

- 🧾 **Metadata Service:**  
  Stores structured product and fashion data for recommendations and analytics.

---

## 🚀 Future Features

- Outfit recommendation and virtual wardrobe  
- AI-based fashion advice and similarity search  
- REST & gRPC APIs for integration with e-commerce platforms  
- Admin dashboard for data insights

---

## ⚙️ Tech Stack

- **Backend:** Go (gRPC, PostgreSQL)  
- **AI / CV:** Python (PyTorch, Transformers, OpenCV)  
- **Frontend / Bot:** Telegram API, Web Frontend (TBD)  
- **Deployment:** Docker, Supabase

---

## 🧠 Vision

AI Stylist aims to become a full-fledged **AI-driven digital stylist**, capable of understanding your wardrobe, recommending looks, and providing data-driven fashion insights.

---

### 🪄 Stay tuned!
Development is ongoing — updates will be published here and on the [Telegram bot](https://t.me/CapsuleMeBot).
