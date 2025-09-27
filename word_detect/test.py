import torch
import joblib
import numpy as np
from transformers import BertForSequenceClassification, BertTokenizer
from sklearn.preprocessing import LabelEncoder

model_path = './final_model'
tokenizer = BertTokenizer.from_pretrained(model_path)
model = BertForSequenceClassification.from_pretrained(model_path)
model.eval()  

le = joblib.load('label_encoder.pkl') 

def predict(texts, model, tokenizer):

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
    
    return decoded_labels[0] if len(texts) == 1 else decoded_labels

if __name__ == '__main__':
    while True:
        sample_text = input("请输入文本：")
        if sample_text=='Q':
            break
        print(f"{predict(sample_text, model, tokenizer)}")



