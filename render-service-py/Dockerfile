FROM python:3.13-slim
RUN apt-get update && apt-get install -y --no-install-recommends fonts-noto-cjk poppler-utils && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py reference_extract.py pdf_extract.py teaching_plots.py science_visuals.py general_scene.py .
RUN useradd --uid 1000 --create-home renderer
USER renderer
ENV MPLCONFIGDIR=/tmp/matplotlib
CMD ["python","-m","uvicorn","app:app","--host","0.0.0.0","--port","8000"]
