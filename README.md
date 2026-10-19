# go-chatbot

A robust, context-aware, and production-ready chatbot service built with **Go (Fiber)**, **PostgreSQL**, and high-performance **Google Gemini AI models**. This application supports both stateless real-time streaming (via Server-Sent Events) and stateful, multi-turn conversations with history persisted securely in PostgreSQL using unique UUIDs.

---

## Features
- **Google Gemini Integration**: Powered by Google's latest Gemini AI models for lightning-fast and intelligent responses.
- **Stateless Streaming Endpoint (`/chat`)**: Real-time token streaming using Server-Sent Events (SSE) with built-in token usage logging and safety ratings.
- **Context-Aware Endpoint (`/chat/context`)**: Stateful multi-turn conversation support that loads prior message history dynamically from a PostgreSQL database using unique chat UUIDs.
- **Containerized Architecture**: Fully dockerized setup running both the Go backend and PostgreSQL in isolated containers via Docker Compose.

---

## Steps to Get a Gemini API Key from Google AI Studio
1. Navigate to [Google AI Studio](https://aistudio.google.com/).
2. Sign in using your Google account.
3. Click on **Get API key** in the top-left corner or navigation menu.
4. Click **Create API key** (you can select an existing Google Cloud project or generate one automatically).
5. Copy your newly created API key and keep it secure.

---

## Environment Configuration (`.env`)

Create a `.env` file in the root directory of your project and add the following fields:

```env
GEMINI_API_KEY="your_actual_gemini_api_key_here"
DB_USER="postgres"
DB_PASSWORD="your_secure_password"
DB_NAME="chatbot_db"
```

---

## Steps to Run

1. **Clone the repository:**
   ```bash
   git clone https://github.com/your-username/go-chatbot.git
   cd go-chatbot
   ```

2. **Add the `.env` file:**
   Create and populate your `.env` file in the project root folder as shown in the configuration section above.

3. **Run Docker Compose:**
   Build and start the application containers in detached mode:
   ```bash
   docker compose up --build -d
   ```

4. **Check running containers:**
   Verify that both the application (`chatbot_app`) and database (`chatbot_postgres`) containers are up and running:
   ```bash
   docker ps
   ```

5. **Hit the endpoints:**
   Use tools like **Bruno**, **Postman**, or `curl` to test your API routes:
   * **Stateless Streaming (`POST /chat`):**
     ```json
     {
       "message": "Hello, write a short snippet about Go."
     }
     ```
   * **Context-Aware (`POST /chat/context`):**
     *(Omit `chatId` for a fresh conversation, or pass a valid UUID to maintain context across turns)*
     ```json
     {
       "chatId": "",
       "message": "Hi, remember that my favorite language is Go."
     }
     ```

6. **Stop the application:**
   When you are done testing or want to spin down the stack, run:
   ```bash
   docker compose down
   ```