from fastapi import FastAPI
from pydantic import BaseModel
import requests  # To proxy to other APIs
from fastapi.middleware.cors import CORSMiddleware  # For production CORS

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Restrict in full production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

class Risk(BaseModel):
    id: str
    name: str

@app.get("/risks")
def get_risks():
    # Proxy to OpenRMF
    return requests.get("http://openrmf:8080/api/risks").json()

@app.get("/threats")
def get_threats():
    # Proxy to OpenCTI with auth
    pass  

@app.get("/incidents")
def get_incidents():
    pass

@app.get("/actions")
def get_actions():
    pass