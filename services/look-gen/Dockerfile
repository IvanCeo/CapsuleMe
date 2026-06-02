FROM python:3.13.2-slim
WORKDIR /look-gen
COPY . .
RUN pip install --upgrade pip
RUN pip install --no-cache-dir -r requirements.txt
CMD ["python", "main.py"]