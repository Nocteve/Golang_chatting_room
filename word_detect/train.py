import torch
import pandas as pd
import numpy as np
from sklearn.preprocessing import LabelEncoder
from sklearn.model_selection import train_test_split
from sklearn.metrics import f1_score, classification_report
from transformers import (
    BertTokenizer,
    BertForSequenceClassification,
    TrainingArguments,
    Trainer,
    EarlyStoppingCallback
)
from datasets import Dataset

def load_data(file_path):
    df = pd.read_csv(file_path)
    texts = df['text'].tolist()
    labels = df['labels'].astype(str).tolist() 
    return texts, labels

# 标签映射
label_names = ['不文明用语', '违法信息', '广告', '正常']  

texts, raw_labels = load_data('dataset.csv')
le = LabelEncoder()
labels = le.fit_transform(raw_labels)

# 数据集划分
train_texts, val_texts, train_labels, val_labels = train_test_split(
    texts, labels, test_size=0.2, random_state=42
)

tokenizer = BertTokenizer.from_pretrained('./bert-base-chinese')

def encode_texts(texts, labels=None, max_length=128):
    """将文本转换为BERT输入格式"""
    encodings = tokenizer(
        texts, 
        max_length=max_length,
        truncation=True,
        padding='max_length',
        return_tensors='pt'
    )
    if labels is not None:
        encodings['labels'] = torch.tensor(labels,dtype=torch.long)
    return encodings

class TextDataset(torch.utils.data.Dataset):
    def __init__(self, encodings):
        self.encodings = encodings
        
    def __getitem__(self, idx):
        return {key: val[idx] for key, val in self.encodings.items()}
        
    def __len__(self):
        return len(self.encodings['input_ids'])

train_encodings = encode_texts(train_texts, train_labels)
val_encodings = encode_texts(val_texts, val_labels)
train_dataset = TextDataset(train_encodings)
val_dataset = TextDataset(val_encodings)

model = BertForSequenceClassification.from_pretrained(
    './bert-base-chinese',
    num_labels=len(label_names),
    problem_type="single_label_classification",  # 指定单标签任务
    output_attentions=False
)

training_args = TrainingArguments(
    output_dir='./results',
    num_train_epochs=15,                         # 训练轮次
    per_device_train_batch_size=16,             # 训练批次
    per_device_eval_batch_size=32,              # 评估批次
    learning_rate=2e-5,                         # 学习率
    eval_strategy='epoch',                # 每轮评估
    save_strategy='epoch',
    logging_steps=100,
    load_best_model_at_end=True,
    metric_for_best_model='f1_micro',           # 按F1选择最佳模型
    fp16=True if torch.cuda.is_available() else False  # 混合精度加速
)

def compute_metrics(p):
    preds = p.predictions  # 模型输出的logits（形状：[样本数, 类别数]）
    labels = p.label_ids   # 真实标签（整数编码，形状：[样本数]）
    
    # 单标签处理：对logits应用softmax，取最大概率的类别索引（整数）
    preds = torch.argmax(torch.softmax(torch.tensor(preds), dim=1), dim=1).numpy()
    
    # 计算F1分数（单标签场景常用macro/micro/weighted）
    f1_micro = f1_score(labels, preds, average='micro')
    f1_macro = f1_score(labels, preds, average='macro')
    
    # 生成详细分类报告
    report = classification_report(labels, preds, target_names=label_names, zero_division=0)
    with open('classification_report.txt', 'w') as f:
        f.write(report)
    
    return {'f1_micro': f1_micro, 'f1_macro': f1_macro}  

trainer = Trainer(
    model=model,
    args=training_args,
    train_dataset=train_dataset,
    eval_dataset=val_dataset,
    compute_metrics=compute_metrics,
    callbacks=[EarlyStoppingCallback(early_stopping_patience=2)]
)

trainer.train()

model.save_pretrained('./final_model')
tokenizer.save_pretrained('./final_model')

def predict(text, model, tokenizer, threshold=0.5):
    device = model.device
    inputs = tokenizer(
        text, 
        max_length=128, 
        truncation=True,
        return_tensors="pt" 
    ).to(device)
    
    with torch.no_grad():
        outputs = model(**inputs)
    
    preds = torch.argmax(torch.softmax(outputs.logits, dim=1), dim=1).cpu().numpy()
    return le.inverse_transform(preds)[0]  # 转回文本标签
# 测试
sample_text = "你好"
print(predict(sample_text, model, tokenizer)) 
import joblib
joblib.dump(le, 'label_encoder.pkl')# 保存编码器
