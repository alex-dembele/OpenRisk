from fastapi import FastAPI
from pydantic import BaseModel
import requests
from fastapi.middleware.cors import CORSMiddleware  # For production CORS

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Restrict in full production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Routes as before, with timeouts and error handling