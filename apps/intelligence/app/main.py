from dataclasses import asdict
from fastapi import FastAPI
from pydantic import BaseModel
from detectors import statistical as st
from diagnostics.rules import diagnose

app = FastAPI(title="Reforge Intelligence", version="0.1.0")


class Series(BaseModel):
    name: str = "metric"
    values: list[float]
    limit: float | None = None


class DiagnoseReq(BaseModel):
    context: dict


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/v1/detect")
def detect(s: Series):
    out = [st.zscore_spike(s.values, name=s.name), st.gradual_growth(s.values, name=s.name)]
    if s.limit:
        out.append(st.threshold(s.values, s.limit, s.name))
    return [asdict(d) for d in out]


@app.post("/v1/diagnose")
def diag(req: DiagnoseReq):
    return asdict(diagnose(req.context))
