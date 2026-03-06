package com.example;

import java.util.Date;

public class Order {
    private Long id;
    private Date orderDate;
    private Double totalAmount;
    private Product product;

    public Long getId() { return id; }
    public void setId(Long id) { this.id = id; }
    public Date getOrderDate() { return orderDate; }
    public void setOrderDate(Date orderDate) { this.orderDate = orderDate; }
    public Double getTotalAmount() { return totalAmount; }
    public void setTotalAmount(Double totalAmount) { this.totalAmount = totalAmount; }
    public Product getProduct() { return product; }
    public void setProduct(Product product) { this.product = product; }
}
