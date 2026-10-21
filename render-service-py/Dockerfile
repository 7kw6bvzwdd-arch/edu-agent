FROM python:3.13-slim
RUN apt-get update && apt-get install -y --no-install-recommends fonts-noto-cjk poppler-utils nodejs npm chromium && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY render-service-py/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN npm install --omit=dev --no-audit --no-fund puppeteer-core@25.14.0
COPY render-service-py/app.py render-service-py/reference_extract.py render-service-py/pdf_extract.py render-service-py/teaching_plots.py render-service-py/science_visuals.py render-service-py/carbon_visuals.py render-service-py/general_scene.py render-service-py/knowledge_images.py render-service-py/native_animation.py render-service-py/authored_page.py .
COPY animation-video/authored-page.mjs animation-video/verify-authored.mjs ./
RUN useradd --uid 1000 --create-home renderer
USER renderer
ENV MPLCONFIGDIR=/tmp/matplotlib
ENV AUTHORED_PAGE_BRIDGE=/app/authored-page.mjs CHROME_PATH=/usr/bin/chromium CONTAINER_BROWSER=1 AUTHORED_ANIMATION_BRIDGE=/app/verify-authored.mjs
CMD ["python","-m","uvicorn","app:app","--host","0.0.0.0","--port","8000"]
