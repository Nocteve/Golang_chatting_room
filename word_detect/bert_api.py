import torch
import joblib
import numpy as np
from transformers import BertForSequenceClassification, BertTokenizer
from sklearn.preprocessing import LabelEncoder
from fastapi import FastAPI
from pydantic import BaseModel

model_path = './final_model'
tokenizer = BertTokenizer.from_pretrained(model_path)
model = BertForSequenceClassification.from_pretrained(model_path)
model.eval()  

le = joblib.load('label_encoder.pkl') 

app=FastAPI()

class Request(BaseModel):
    text:str

class Response(BaseModel):
    embeddings:str

@app.post("/predict", response_model=Response)
async def predict(request_body: Request):
    texts=request_body.text
    if isinstance(texts, str):
        texts = [texts]
        
    
    inputs = tokenizer(
        texts,
        max_length=128,
        truncation=True,
        padding='max_length',
        return_tensors="pt"
    ).to(model.device)  
    
    with torch.no_grad():
        outputs = model(** inputs)
        logits = outputs.logits  
        
        probs = torch.softmax(logits, dim=1)  
        preds = torch.argmax(probs, dim=1).cpu().numpy()  
    
    decoded_labels = le.inverse_transform(preds)  
    embeddings=decoded_labels[0] if len(texts) == 1 else decoded_labels
    return {"embeddings": embeddings}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)



