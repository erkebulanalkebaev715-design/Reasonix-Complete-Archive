import unittest
from reasonix_model.train import train_smoke

class TrainingTests(unittest.TestCase):
    def test_training_loss_drops(self):
        corpus=("abc abc abc reasonix mobile model. "*30)
        r=train_smoke(corpus,steps=18,seq=16,batch=2,lr=4e-3)
        self.assertTrue(r["loss_decreased"], (r["initial_loss"],r["final_loss"]))
